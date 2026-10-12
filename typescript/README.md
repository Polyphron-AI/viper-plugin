# @polyphron/viper-plugin

Build [Viper](https://github.com/Polyphron-AI/viper) plugins in TypeScript.
`serve()` does the handshake, checks the token both ways, turns JSON bytes
into values and stops when Viper closes stdin. Every Host call is typed and
awaitable on `c.host`.

```ts
import { json, serve } from "@polyphron/viper-plugin";

await serve({
  description: { id: "hello", version: "1.0.0", title: "Hello", routes: [{ method: "GET", path: "hi" }] },
  routes: { "GET hi": () => json(200, { message: "Hi!" }) },
});
```

Compile to one binary per platform with `bun build --compile`, then sign it.
The full walk-through is in the
[viper-plugin README](https://github.com/Polyphron-AI/viper-plugin#readme).

MIT licence.
