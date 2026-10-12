// @polyphron/viper-plugin: build a Viper plugin in TypeScript. Fill in a
// Plugin and call serve(); it does the handshake, checks the token both ways,
// turns JSON bytes into values and stops when Viper closes stdin. Compile the
// result to one program per platform, for example with `bun build --compile`.
import * as grpc from "@grpc/grpc-js";

import {
  HostClient,
  HostService,
  PluginService,
  type Call,
  type Description,
  type EventRequest,
  type RouteReply,
  type RouteRequest,
} from "./gen/plugin.js";

export * from "./gen/plugin.js";

/** Handshake is the first word of the line a plugin prints once it serves. */
export const Handshake = "VIPER-PLUGIN";

/** Protocol is the handshake version this kit speaks. */
export const Protocol = 1;

type HostName = keyof typeof HostService;
type Req<K extends HostName> = ReturnType<(typeof HostService)[K]["requestDeserialize"]>;
type Rep<K extends HostName> = ReturnType<(typeof HostService)[K]["responseDeserialize"]>;

/**
 * Host is every Host call, awaitable. The call id is filled in for you, so a
 * request acts in the call's project as the call's caller (or as the service
 * on an elevated route).
 */
export type Host = {
  [K in HostName]: (request: Omit<Req<K>, "call">) => Promise<Rep<K>>;
};

/** BlockResult is a step's output and the way out ("" or left out for the default). */
export interface BlockResult {
  output?: unknown;
  way?: string;
}

type Maybe<T> = T | Promise<T>;

/** Plugin is everything a plugin declares and does. */
export interface Plugin {
  description: Description;
  /** Blocks are keyed by name. */
  blocks?: Record<string, (c: PluginCall, config: Record<string, unknown>) => Maybe<BlockResult>>;
  /** Tools are keyed by name. A thrown error goes back to the agent as an error result. */
  tools?: Record<string, (c: PluginCall, args: Record<string, unknown>) => Maybe<unknown>>;
  /** Routes are keyed by "METHOD path", like "GET items/:id". */
  routes?: Record<string, (c: PluginCall, r: RouteRequest) => Maybe<RouteReply>>;
  event?: (c: PluginCall, e: EventRequest) => Maybe<void>;
  /** Jobs are keyed by kind. */
  jobs?: Record<string, (c: PluginCall, payload: Buffer) => Maybe<void>>;
}

/** PluginCall is one call from Viper, with the Host bound to it. */
export class PluginCall {
  readonly call: Call;
  readonly host: Host;

  constructor(call: Call | undefined, host: (call: string) => Host) {
    this.call = call ?? {};
    this.host = host(this.call.id ?? "");
  }

  /** settings decodes the project's settings for this plugin. */
  settings<T = Record<string, unknown>>(): T {
    return fromJSON(this.call.settings) as T;
  }

  /** list reads records from a collection with an optional filter. */
  async list<T = Record<string, unknown>>(collection: string, filter = ""): Promise<T[]> {
    const out = await this.host.listRecords({ collection, filter });

    const items = fromJSON(out.items);

    return Array.isArray(items) ? (items as T[]) : [];
  }

  /** create adds a record to a collection and returns it. */
  async create<T = Record<string, unknown>>(collection: string, body: unknown): Promise<T> {
    const out = await this.host.createRecord({ collection, body: toJSON(body) });

    return fromJSON(out.record) as T;
  }

  /**
   * accessToken returns a user's token at a connected account (like
   * "google") for scopes, and when it expires. The manifest's connections
   * and the project's grant must name them. A user's call may ask only for
   * that user; a job or event (with background) only for a user its payload
   * names.
   */
  async accessToken(user: string, provider: string, scopes: string[]): Promise<{ token: string; expires: Date }> {
    const out = await this.host.getAccessToken({ user, provider, scopes });

    return { token: out.token ?? "", expires: new Date((out.expiresUnix ?? 0) * 1000) };
  }

  /** log writes a line to Viper's log. */
  async log(level: string, message: string): Promise<void> {
    await this.host.log({ level, message });
  }
}

/** toJSON is a value as the JSON bytes the contract carries. */
export function toJSON(v: unknown): Buffer {
  return Buffer.from(JSON.stringify(v ?? null));
}

/** fromJSON reads JSON bytes; empty bytes are an empty object. */
export function fromJSON(b?: Uint8Array): unknown {
  return b && b.length ? JSON.parse(Buffer.from(b).toString()) : {};
}

/** json is a route reply with a JSON body. */
export function json(status: number, v: unknown): RouteReply {
  return {
    status,
    headers: { "Content-Type": "application/json" },
    body: toJSON(v),
  };
}

/** PluginError carries a gRPC status code, so Viper picks the HTTP status. */
export class PluginError extends Error {
  readonly code: grpc.status;

  constructor(code: grpc.status, message: string) {
    super(message);
    this.code = code;
  }
}

/** fail is an error Viper shows as is, with a 400, instead of a generic failure. */
export function fail(message: string): PluginError {
  return new PluginError(grpc.status.INVALID_ARGUMENT, message);
}

/** notFound is an error Viper answers with a 404 and the message. */
export function notFound(message: string): PluginError {
  return new PluginError(grpc.status.NOT_FOUND, message);
}

/**
 * asStatus turns a thrown error into a gRPC status. A failed Host call
 * keeps its own code and message (a refused write stays a 403), the way a
 * Go plugin returning the error does.
 */
function asStatus(err: unknown): Partial<grpc.ServiceError> {
  if (err instanceof PluginError) return { code: err.code, details: err.message };

  const e = err as Partial<grpc.ServiceError>;
  if (typeof e?.code === "number" && typeof e.details === "string") {
    return { code: e.code, details: e.details };
  }

  return { code: grpc.status.UNKNOWN, details: err instanceof Error ? err.message : String(err) };
}

/**
 * serve starts the plugin: it serves the Plugin service on a loopback port,
 * prints the handshake line and resolves once Viper closes stdin and the
 * server has stopped.
 */
export function serve(p: Plugin): Promise<void> {
  const token = process.env.VIPER_PLUGIN_TOKEN;
  if (!token) {
    return Promise.reject(new Error("this is a Viper plugin; Viper starts it"));
  }

  const auth = `Bearer ${token}`;
  const client = new HostClient(
    process.env.VIPER_PLUGIN_HOST ?? "",
    grpc.credentials.createInsecure(),
  );

  const host = (call: string): Host => {
    const out: Record<string, unknown> = {};

    for (const name of Object.keys(HostService)) {
      out[name] = (request: object) => {
        const md = new grpc.Metadata();
        md.set("authorization", auth);

        return new Promise((resolve, reject) =>
          (client as unknown as Record<string, Function>)[name].call(
            client,
            { ...request, call },
            md,
            (err: Error | null, reply: unknown) => (err ? reject(err) : resolve(reply)),
          ),
        );
      };
    }

    return out as Host;
  };

  // guard refuses a call without our token, so no other local program can
  // call the plugin, and turns a thrown error into a gRPC one.
  const guard =
    <Q, R>(fn: (req: Q) => Maybe<R>) =>
    async (call: grpc.ServerUnaryCall<Q, R>, done: grpc.sendUnaryData<R>) => {
      if (call.metadata.get("authorization")[0] !== auth) {
        return done({ code: grpc.status.UNAUTHENTICATED, details: "wrong plugin token" });
      }

      try {
        done(null, await fn(call.request));
      } catch (err) {
        done(asStatus(err));
      }
    };

  const missing = (what: string) => new PluginError(grpc.status.NOT_FOUND, `no ${what}`);
  const ctx = (call?: Call) => new PluginCall(call, host);

  const server = new grpc.Server({ "grpc.max_receive_message_length": 64 << 20 });

  server.addService(PluginService, {
    describe: guard(() => p.description),

    runBlock: guard(async (r: { call?: Call; block?: string; config?: Buffer }) => {
      const run = p.blocks?.[r.block ?? ""];
      if (!run) throw missing(`block "${r.block}"`);

      const out = await run(ctx(r.call), fromJSON(r.config) as Record<string, unknown>);

      return { output: toJSON(out.output), way: out.way ?? "" };
    }),

    callTool: guard(async (r: { call?: Call; tool?: string; arguments?: Buffer }) => {
      const run = p.tools?.[r.tool ?? ""];
      if (!run) throw missing(`tool "${r.tool}"`);

      try {
        return { result: toJSON(await run(ctx(r.call), fromJSON(r.arguments) as Record<string, unknown>)) };
      } catch (err) {
        return { result: toJSON(err instanceof Error ? err.message : String(err)), isError: true };
      }
    }),

    handleRoute: guard(async (r: RouteRequest) => {
      const run = p.routes?.[`${r.method} ${r.route}`];
      if (!run) return { status: 404 };

      return run(ctx(r.call), r);
    }),

    handleEvent: guard(async (r: EventRequest) => {
      await p.event?.(ctx(r.call), r);

      return {};
    }),

    runJob: guard(async (r: { call?: Call; kind?: string; payload?: Buffer }) => {
      const run = p.jobs?.[r.kind ?? ""];
      if (!run) throw missing(`job kind "${r.kind}"`);

      await run(ctx(r.call), r.payload ?? Buffer.alloc(0));

      return {};
    }),
  });

  return new Promise((resolve, reject) => {
    server.bindAsync("127.0.0.1:0", grpc.ServerCredentials.createInsecure(), (err, port) => {
      if (err) return reject(err);

      process.stdout.write(`${Handshake} ${Protocol} 127.0.0.1:${port}\n`);
    });

    // Viper holds our stdin open while it wants us running. The end means it
    // stopped us or died, so a plugin never outlives its Viper.
    process.stdin.on("end", () =>
      server.tryShutdown(() => {
        client.close();
        resolve();
      }),
    );
    process.stdin.resume();
  });
}
