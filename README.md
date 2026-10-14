# viper-plugin

Build plugins for [Viper](https://github.com/Polyphron-AI/viper) in Go or
TypeScript. A plugin is its own program. Viper starts it, talks to it over
gRPC, and the plugin calls Viper back to read and write records, send mail,
save files and more, always as the user who called it.

| Part | Where | Use it for |
|---|---|---|
| The contract | [`proto/plugin.proto`](proto/plugin.proto) | Any language with gRPC |
| Go SDK | module `github.com/Polyphron-AI/viper-plugin` (`kit`, `proto`, `archive`) | Go plugins |
| TypeScript SDK | npm [`@polyphron/viper-plugin`](typescript) | TypeScript plugins, compiled with Bun |
| Signing tool | `cmd/viper-plugin` (`keygen`, `sign`) | Making a `.vplugin` archive |
| Examples | [`examples/drive-files`](examples/drive-files) (Go: a user's Google Drive files), [`examples/chat-alerts`](examples/chat-alerts) (TypeScript: new records to Slack, Discord or Teams) | Starting points to copy |

The full guide (the five rules, errors, elevated routes, upload sessions) is
[GUIDE.md](GUIDE.md).

## A Go plugin

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/Polyphron-AI/viper-plugin@latest
```

`main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/Polyphron-AI/viper-plugin/kit"
	pb "github.com/Polyphron-AI/viper-plugin/proto"
)

func main() {
	err := kit.Serve(&kit.Plugin{
		Description: &pb.Description{
			Id:      "hello",
			Version: "1.0.0",
			Title:   "Hello",
			Routes:  []*pb.Route{{Method: "GET", Path: "hi", Summary: "Says hi."}},
		},
		Routes: map[string]kit.RouteFunc{
			"GET hi": func(c *kit.Call, r *pb.RouteRequest) (*pb.RouteReply, error) {
				c.Log("info", "said hi")

				return kit.JSON(200, map[string]string{"message": "Hi!"})
			},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Build one binary per platform you support:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/hello-linux-amd64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/hello.exe .
```

## A TypeScript plugin

```sh
mkdir hello && cd hello
bun init -y
bun add @polyphron/viper-plugin
```

`index.ts`:

```ts
import { json, serve } from "@polyphron/viper-plugin";

await serve({
  description: {
    id: "hello",
    version: "1.0.0",
    title: "Hello",
    routes: [{ method: "GET", path: "hi", summary: "Says hi." }],
  },
  routes: {
    "GET hi": async (c) => {
      await c.log("info", "said hi");

      return json(200, { message: "Hi!" });
    },
  },
});
```

Every Host call is on `c.host`, typed and awaitable, with the call id filled
in: `await c.host.readSecret({ name: "API_KEY" })`. Throw `fail("...")` for a
400 with your message.

Viper runs the binary directly, so compile it to one file per platform:

```sh
bun build index.ts --compile --target=bun-linux-x64 --outfile bin/hello-linux-amd64
bun build index.ts --compile --target=bun-windows-x64 --outfile bin/hello.exe
```

## Sign and install

Next to the binaries, write `manifest.json`. The `id` and `version` must
match the plugin's description. `viper` is the range of Viper versions it
runs on; a server outside it refuses the install, and leaving it out means
any 0.6 release. `sign --viper` sets it too. Platforms are named `os/arch`,
the Go way:

```json
{
  "id": "hello",
  "version": "1.0.0",
  "title": "Hello",
  "viper": ">=0.6.8 <0.7.0",
  "binaries": {
    "linux/amd64": { "path": "bin/hello-linux-amd64" },
    "windows/amd64": { "path": "bin/hello.exe" }
  }
}
```

Then make a key once and sign:

```sh
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest keygen --out plugin.key
go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin@latest sign --key plugin.key .
```

Keep `plugin.key` secret and out of git. `keygen` prints the public key: the
server trusts it through `VIPER_PLUGIN_KEYS`. A dev server (`VIPER_DEV`) also
takes unsigned archives: leave out `--key`.

Install the `.vplugin` file on the server's **Plugins** page, turn it on in a
project, and call it at `GET /api/p/<project>/plugins/hello/hi`.

## Versions

The SDK's version matches the Viper release it was made with. Fields in
`plugin.proto` are only added, never renamed or removed, so a plugin built
with an older SDK keeps working on a newer Viper.

## Licence

MIT. See [LICENSE](LICENSE).
