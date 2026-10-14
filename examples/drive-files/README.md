# Drive files (Go)

An app route that lists the signed-in user's newest Google Drive files. It
asks Viper for the user's Google token with `c.AccessToken`, so the user must
have connected Google Drive in your app first.

```
GET /api/p/<project>/plugins/drivefiles/files
→ 200 {"files": [{"id": "...", "name": "Plan.pdf", "mimeType": "application/pdf"}]}
→ 409 {"message": "Connect Google Drive first.", "connect": "google"}
→ 401 for a visitor
```

## Before you start

In the project, on **Auth**, set up the Google provider and put
`https://www.googleapis.com/auth/drive.readonly` in its **Connect scopes**.
Then your app connects a user:

```ts
const { url } = await viper.connections.start(
  "google",
  ["https://www.googleapis.com/auth/drive.readonly"],
  "https://your.app/settings",
);
location.assign(url);
```

## Build, sign and install

Run these in this folder. Make a signing key once (keep `plugin.key` secret):

```sh
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest keygen --out plugin.key
```

Build one binary per platform the manifest lists:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/drive-files-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/drive-files-linux-arm64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/drive-files-darwin-arm64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/drive-files.exe .
```

Sign them into `drivefiles-1.0.0.vplugin`:

```sh
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest sign --key plugin.key .
```

Put the public key `keygen` printed into the server's `VIPER_PLUGIN_KEYS`,
upload the archive on the server's **Plugins** page, and turn it on in a
project. The confirm lists `google: .../drive.readonly`: that is the access
you grant.

## Start your own from it

Copy this folder out, then make it its own module:

```sh
go mod init example.com/drive-files
go get github.com/Polyphron-AI/viper-plugin@latest
```

Change the `id` in `main.go` and `manifest.json` together, and ask only for
the scopes you need.
