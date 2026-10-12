// Chat alerts: an example Viper plugin. When a record in a watched table is
// created (or updated or deleted, per the project's settings), it posts one
// line to a Slack, Discord or Teams incoming webhook.
import { fromJSON, serve, toJSON, type EventRequest, type PluginCall } from "@polyphron/viper-plugin";

// WATCH is the tables it gets events for. Viper reads this list when the
// plugin is installed, so change it here, not per project.
const WATCH = ["orders"];

/** Settings is what each project sets on the plugin's page. */
type Settings = {
  webhookUrl?: string;
  service?: "slack" | "discord" | "teams";
  actions?: string;
};

const verbs: Record<string, string> = { create: "New", update: "Updated", delete: "Deleted" };

/**
 * alert posts one message for an event. Viper may send an event again (a
 * retry after a failure), always with the same delivery id, so a delivery
 * already posted is skipped.
 */
async function alert(c: PluginCall, e: EventRequest): Promise<void> {
  const s = c.settings<Settings>();
  const actions = (s.actions || "create").split(",").map((a) => a.trim());
  const action = e.action ?? "";

  if (!s.webhookUrl || !actions.includes(action)) return;

  // The delivery id is a number, so JSON quoting is a safe filter string.
  const delivery = c.call.delivery ?? "";
  const sent = await c.list("chatalerts_sent", `delivery = ${JSON.stringify(delivery)}`);

  if (sent.length > 0) return;

  const record = fromJSON(e.record) as Record<string, unknown>;
  const label = record.title ?? record.name ?? record.id;
  const text = `${verbs[action]} ${e.collection}: ${label}`;
  const res = await fetch(s.webhookUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(s.service === "discord" ? { content: text } : { text }),
    signal: AbortSignal.timeout(15_000),
  });

  // A failure throws, so Viper tries the event again later.
  if (!res.ok) throw new Error(`the ${s.service || "slack"} webhook answered ${res.status}`);

  // ponytail: a crash between the post and this line posts twice on the
  // retry; chat webhooks take no idempotency key, so this is the floor.
  await c.create("chatalerts_sent", { delivery });
}

await serve({
  description: {
    id: "chatalerts",
    version: "1.0.0",
    title: "Chat alerts",
    events: WATCH.map((collection) => ({ collection, actions: [] })),
    settings: [
      { key: "webhookUrl", label: "Webhook URL", kind: "secret", required: true, description: "The incoming webhook's address." },
      {
        key: "service",
        label: "Chat app",
        kind: "select",
        options: [
          { id: "slack", label: "Slack" },
          { id: "discord", label: "Discord" },
          { id: "teams", label: "Microsoft Teams" },
        ],
        defaultJson: '"slack"',
      },
      { key: "actions", label: "Post on", kind: "text", description: "create, update, delete; separated by commas.", defaultJson: '"create"' },
    ],
    // One row per posted delivery, so a repeat posts nothing.
    collections: toJSON([
      { name: "chatalerts_sent", type: "base", fields: [{ name: "delivery", type: "text", unique: true }] },
    ]),
  },
  event: alert,
});
