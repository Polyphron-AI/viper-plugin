# Chat alerts (TypeScript)

Posts one line to a Slack, Discord or Microsoft Teams incoming webhook when a
record is made in a watched table. Each project sets its own webhook and
which actions post (`create`, `update`, `delete`).

```
New orders: Blue mug
```

Viper may send an event again after a failure. The plugin keeps one row per
posted delivery in its `chatalerts_sent` table, so a repeat posts nothing.

## Pick the tables

The tables it watches are `WATCH` in `index.ts` (`orders` to start). Viper
reads that list when the plugin is installed, so change it there and build
again; a project can't pick its own.

## Build, sign and install

Run these in this folder. You need [Bun](https://bun.sh). Make a signing key
once (keep `plugin.key` secret):

```sh
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest keygen --out plugin.key
```

Install the SDK and build one binary per platform the manifest lists:

```sh
bun install
bun build index.ts --compile --target=bun-linux-x64 --outfile bin/chat-alerts-linux-amd64
bun build index.ts --compile --target=bun-windows-x64 --outfile bin/chat-alerts.exe
```

Sign them into `chatalerts-1.0.0.vplugin`:

```sh
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest sign --key plugin.key .
```

Put the public key `keygen` printed into the server's `VIPER_PLUGIN_KEYS`,
upload the archive on the server's **Plugins** page (a Bun binary is about
100 MB, so the page sends it in parts), turn it on in a project, and fill in
**Webhook URL** and **Chat app**.

## Start your own from it

Copy this folder out and change the `id` in `index.ts` and `manifest.json`
together. Add a platform by adding a `bun build` line and its entry under
`binaries` in `manifest.json`.
