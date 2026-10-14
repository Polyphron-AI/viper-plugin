# Writing a Viper plugin

A plugin can be written in any language that speaks gRPC. Go uses the `kit`
package and TypeScript uses `@polyphron/viper-plugin` (both in this repo,
see the [README](README.md)); every other language follows
[`proto/plugin.proto`](proto/plugin.proto) directly.

## What a plugin must do

Whatever the language, a plugin keeps five rules. Both SDKs keep them for
you; `kit/kit.go` and `typescript/src/index.ts` show how.

1. **Be one program per platform.** Viper runs the binary directly, with no
   interpreter, so a script won't do. Compile it to a single file (Bun's
   `--compile`, Deno's `compile`, a Node single executable app, or any
   compiled language).
2. **Read two variables.** Viper sets `VIPER_PLUGIN_TOKEN` and
   `VIPER_PLUGIN_HOST` (the address of Viper's `Host` service). It passes
   only a few others (`PATH`, `TEMP`, `HOME` and so on), never Viper's own
   secrets.
3. **Serve the `Plugin` service on `127.0.0.1`.** Pick port 0 so the system
   gives a free one, then print one line to stdout within 15 seconds:
   `VIPER-PLUGIN 1 127.0.0.1:<port>`. Anything printed after that goes to
   Viper's log, and so does stderr.
4. **Check the token both ways.** Every call from Viper carries
   `authorization: Bearer <token>`; refuse others with `UNAUTHENTICATED`.
   Send the same header on every call to `Host`.
5. **Stop when stdin closes.** Viper holds the plugin's stdin open while it
   wants it running. End of input means stop.

`Describe` must return the same `id` and `version` as the signed
`manifest.json`, or Viper refuses to start it.

Values in records, settings, step configs, tool arguments and job payloads
travel as JSON in `bytes` fields, so no language needs protobuf's well-known
types.

## Errors

Return a gRPC status. Viper keeps the message and picks the HTTP status from
the code:

| gRPC code | Viper answers |
|---|---|
| `INVALID_ARGUMENT`, `FAILED_PRECONDITION` | 400 with your message |
| `NOT_FOUND` | 404 with your message |
| `PERMISSION_DENIED` | 403 with your message |
| `UNAUTHENTICATED` | 401 with your message |
| anything else | 500, logged, with a generic message |

## The Host calls

The [README](README.md) shows a whole plugin in Go and in TypeScript. In Go,
Host calls are on `c.Host` (with helpers like `c.List` and `c.Create`); in
TypeScript they are on `c.host`, awaitable, with the call id filled in.

Things to know about every call:

- The TypeScript SDK names fields in camelCase (`defaultJson`,
  `inputSchema`, `clientIp`); the wire names are the `.proto` ones.
- Every `Host` request names the call it belongs to; the SDKs fill it in.
  It acts in that call's project as that call's caller, so the project's rules
  apply. A call id stops working once the call returns, so finish your `Host`
  calls before you answer.
- `c.settings()` (Go: `c.Settings(&v)`) is the project's saved settings
  for this plugin. Secret settings arrive unsealed.
- Settings, block, tool, job and route names are plain names: a letter, then
  letters, digits or `_`. The id is 2 to 31 lowercase letters and digits.

### Build a TypeScript plugin

Viper names platforms `os/arch` the Go way. Build one binary for each
platform you want to support:

```sh
bun build index.ts --compile --target=bun-windows-x64 --outfile bin/hellots.exe
bun build index.ts --compile --target=bun-linux-x64   --outfile bin/hellots-linux-amd64
bun build index.ts --compile --target=bun-linux-arm64 --outfile bin/hellots-linux-arm64
bun build index.ts --compile --target=bun-darwin-arm64 --outfile bin/hellots-darwin-arm64
```

Bun downloads each target's runtime the first time. On Windows that download
can fail with "Failed to extract executable"; build the Linux and macOS
binaries in WSL or on that OS instead.

### `manifest.json`

```json
{
  "id": "hellots",
  "version": "1.0.0",
  "title": "Hello TS",
  "viper": ">=0.6.8 <1.0.0",
  "binaries": {
    "windows/amd64": { "path": "bin/hellots.exe" },
    "linux/amd64": { "path": "bin/hellots-linux-amd64" },
    "linux/arm64": { "path": "bin/hellots-linux-arm64" },
    "darwin/arm64": { "path": "bin/hellots-darwin-arm64" }
  }
}
```

The `id` and `version` must match the plugin's description. Leave out
`sha256`; signing fills it in. `viper` is the range of Viper versions the
plugin runs on, comparisons split by spaces (`>=`, `<=`, `>`, `<`, `=`). A
server outside it refuses the install; leaving it out means any 0.x release from 0.6 on.
`sign --viper ">=0.6.8 <1.0.0"` sets it without editing the file.

### Sign

```sh
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest keygen --out plugin.key   # once; prints the public key
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest sign --key plugin.key --out hellots.vplugin .
```

`viper plugin keygen` and `viper plugin sign` on a Viper server binary do
the same.

Keep `plugin.key` secret and out of git. Give the public key to the server
in `VIPER_PLUGIN_KEYS` (comma separated for more than one). A server in dev
mode (`VIPER_DEV`) also runs unsigned archives: leave out `--key`.

## Size

A Bun binary is about 100 MB, and about 40 MB inside the zip. An archive may
be up to 256 MB, so four platforms fit. One binary unpacked may be up to
200 MB.

Archives over 10 MB go in parts. The dashboard's **Plugins** page and the
admin SDK's `plugins.install(file)` do that for you. With the raw API, start
an upload with `POST /api/admin/uploads/restore` (`{"size": <bytes>}`), send
each part with `PUT /api/admin/uploads/<id>/parts/<n>`, then
`POST /api/admin/plugins` with `{"upload": "<id>"}`.

## Install and try it

1. As a superuser, install the `.vplugin` file on the server's **Plugins**
   page. It should show as running.
2. In a project's **Plugins** page, turn it on and fill in its settings.
3. Call it:
   - Route: `GET /api/p/<project>/plugins/<id>/<path>`, or from an app,
     `viper.plugin("<id>").request("<path>")`.
   - MCP tool: `<id>_<tool>` in the project's MCP.
   - Workflow step: type `<id>.<block>`.

If it doesn't start, the server's **Plugins** page shows why, and the
plugin's stderr is in Viper's log under `plugin=<id>`.

## Going further

The `hello` plugin in Viper's repo uses every part of the contract: a managed
collection, a secret, admin routes, record events and jobs. Each maps to the
same `.proto` fields here:

| Want | Add to the description | Then |
|---|---|---|
| Its own tables | `collections`: JSON array of collection definitions | Read and write them with `ListRecords`, `CreateRecord`, ... |
| A project secret | `secrets: ["NAME"]` | `c.host.readSecret({ name })` |
| An admin route | a route with `admin: true` | Served at `/api/admin/projects/<project>/plugins/<id>/x/<path>` |
| Record events | `events: [{ collection, actions }]` | Handle it in `event`; it runs as a job, so it never slows a write. See [Retries](#retries) |
| Jobs | `jobs: ["kind"]` | `c.host.enqueueJob({ kind, payload })`, handle it in `jobs` |
| The caller's address | `req.clientIp` and `req.clientIpTrusted` on a route request | See [Client address](#client-address) |
| A big file from a visitor | `Host.CreateUpload` and `Host.CompleteUpload` | See [Upload sessions](#upload-sessions) |
| A route that acts as the service | a route with `elevated: true`, plus a matching entry in the manifest's `elevated` list | See [Elevated routes](#elevated-routes) |

## Retries

An event or job can reach your handler more than once. When a handler
returns an error, Viper tries again after 10s, 20s, 40s and so on, up to an
hour apart, until its tries run out; then the job is dead and stays in
Viper's job list with the last error.

Every try of one event or job carries the same delivery id: `c.Delivery`
in Go, `c.call.delivery` in TypeScript (`""` on routes, tools and blocks).
Write each handler so a second run does no harm: before doing the work,
look for a record that says this delivery is done, and skip it if so.

## Client address

A route request carries `clientIp`: the caller's address as Viper sees it,
the socket peer or the client behind a trusted proxy
(`VIPER_TRUSTED_PROXIES`). IPv4 inside IPv6 comes as IPv4, and it is `""`
if it can't be told. Use it to limit or log by address.

`clientIpTrusted` is false when the request had `X-Forwarded-For`,
`X-Real-IP` or `Forwarded` but Viper trusts no proxy. Then `clientIp` is
most likely the proxy, the same for every visitor, so don't limit on it.

Those headers still show in `headers`, as the client sent them. Never trust
them. An address can be personal data: store a hash or a shortened form
unless you need the whole one.

## Elevated routes

A route runs as whoever called it, so a visitor's call can't write a table
whose rules keep visitors out. An elevated route runs as the service
instead, on only what the signed manifest grants it:

```json
"elevated": [
  { "route": "POST ingest", "collections": ["reports"], "actions": ["create"] }
]
```

- `route` is `"<METHOD> <path>"`, as the route is declared. The route must
  also set `elevated: true`; Viper refuses to start a plugin where the two
  lists differ. Admin routes can't be elevated.
- `actions` are `list`, `view`, `create`, `update`, `delete` and `file`.
  Saving a file (`SaveFile`) needs `file`, plus `view` and `update`, since
  it reads the record and goes through its update rule.
- Up to 32 routes. The manifest is signed, so the plugin can't widen the
  grant at runtime.
- On such a call `call.elevated` is true, and `call.caller` still names who
  called (`""` for a visitor). A `Host` request outside the grant gets
  `PERMISSION_DENIED`. Every write is in the project's audit log as
  `service/<id>`, with the route, the request id and the caller's address.
- A project admin must confirm the elevated routes when turning the plugin
  on (`allowElevated: true`; the dashboard asks).

Viper no longer checks the project's rules for these calls, so the plugin
must check the request itself first: its key, origin and size.

## Connected account tokens

A plugin can work with a user's Google or Microsoft account (Drive files,
Teams messages) once that user has connected it in the app
(`viper.connections.start`). The manifest names what it needs:

```json
"connections": [
  { "provider": "google", "scopes": ["https://www.googleapis.com/auth/drive.readonly"], "background": true }
]
```

Then ask for a token:

```go
// c.Caller is "user/<id>" on a signed-in user's call.
token, expires, err := c.AccessToken(strings.TrimPrefix(c.Caller, "user/"), "google", drive)
```

```ts
const { token, expires } = await c.accessToken(userId, "google", [drive]);
```

- `provider` is `google` or `microsoft`; `scopes` are the ones the token is
  for. The project's provider must allow them as extra scopes too.
- The token is for one user: a user's own call may ask only for that user.
  A job or event may ask, with `background: true`, only for a user its
  payload names (put the user id in the job's payload).
- Anything else gets `PERMISSION_DENIED`: another user, a visitor, a key, a
  provider or scope the manifest leaves out. A user who never connected gets
  `NOT_FOUND`; one whose refresh was refused gets `FAILED_PRECONDITION` and
  must connect again.
- Viper refreshes the token for you. Don't keep it past `expires`.
- Connections are part of the signed grant. A project admin confirms them
  with the elevated routes when turning the plugin on, and an upgrade that
  asks for more waits until they accept it.

## Upload sessions

A route body stops at 10 MB, and `SaveFile` holds the whole file in
memory. For a bigger file, open an upload session and let the visitor send
the bytes themselves:

1. `CreateUpload` with the collection, record, file field, name and size
   (and, if you have them, `sha256`, `contentType` and `ttlSeconds`). Ask
   for `direct: true`.
2. When the project stores files in S3, the reply's `mode` is `"direct"`:
   hand the visitor `url` and `headers`. They `PUT` the whole file there,
   with no key. The URL is signed, expires (`expiresUnix`) and only takes
   exactly `size` bytes.
3. `CompleteUpload` with the `upload` id. Viper checks the size, the sha256
   and the file's real type, then adds it to the record.

With files on disk there is no URL: `mode` is `"parts"` and `url` is
`""`. Then refuse, or take the bytes through your own route.

The session belongs to the plugin in that project, not to the visitor, so
only the plugin can finish it. Starting one goes through the update rule.
A visitor's call needs an [elevated route](#elevated-routes) whose grant
has `file`, `view` and `update` on that collection. The field's `maxSize`
and `mimeTypes` are the limits, and a plugin holds at most 20 open
sessions per project. Unfinished sessions are swept when they expire.

