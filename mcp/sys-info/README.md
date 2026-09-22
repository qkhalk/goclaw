# System Info

Read host stats for agents — OS, uptime, load, memory, per-path disk usage — as clean JSON.

Zero-dependency MCP stdio server (category: system, runtime: node).

## Layout

- `manifest.json` — catalog metadata (name, entry, runtime); ships in the Tool Store
- `src/index.js` — the server; tools: `sys_info`, `disk_usage`
- No test/ folder — verified via the installer smoke test at install time

## Develop

```
(printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n'; sleep 1) | node src/index.js
```

## Ship

Commit this folder under `mcp/sys-info/` and cut a release tag — the
dynamic catalog picks it up for every existing install within ~15 minutes,
no gateway upgrade needed.
