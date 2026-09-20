#!/usr/bin/env node
/**
 * sys-info — zero-dependency MCP stdio server.
 *
 * Tools:
 *   sys_info()                  → OS, arch, uptime, load, memory, hostname
 *   disk_usage({path})          → df -k stats for one path (POSIX hosts)
 *
 * Newline-delimited JSON-RPC 2.0 over stdin/stdout, no SDK dependency.
 */
"use strict";

const os = require("os");
const { execFile } = require("child_process");

const PROTOCOL_VERSION = "2025-06-18";
const DF = process.env.DF_PATH || "df";

const TOOL_SYS_INFO = {
  name: "sys_info",
  description: "Read host stats: OS, architecture, uptime, load average, memory, hostname.",
  inputSchema: { type: "object", properties: {} },
};

const TOOL_DISK_USAGE = {
  name: "disk_usage",
  description: "Read disk usage (total/used/available/pct) for one filesystem path via df.",
  inputSchema: {
    type: "object",
    properties: {
      path: { type: "string", description: "Absolute filesystem path to stat (default /)." },
    },
  },
};

function textResult(text) {
  return { content: [{ type: "text", text }] };
}

function sysInfo() {
  const total = os.totalmem();
  const free = os.freemem();
  return {
    hostname: os.hostname(),
    os: `${os.type()} ${os.release()}`,
    platform: os.platform(),
    arch: os.arch(),
    cpus: os.cpus().length,
    uptime_seconds: Math.round(os.uptime()),
    loadavg: os.loadavg().map((v) => Math.round(v * 100) / 100),
    memory: {
      total_bytes: total,
      free_bytes: free,
      used_pct: total > 0 ? Math.round(((total - free) / total) * 100) : null,
    },
  };
}

/** df -k -P <path> (POSIX output) → last line: fs, blocks, used, avail, pct, mount. */
function diskUsage(path) {
  return new Promise((resolve, reject) => {
    execFile(DF, ["-k", "-P", "--", path], { timeout: 5000 }, (err, stdout, stderr) => {
      if (err) {
        reject(new Error(`df failed: ${(stderr || err.message).trim().slice(-200)}`));
        return;
      }
      const lines = stdout.trim().split("\n");
      if (lines.length < 2) {
        reject(new Error("df output unexpected"));
        return;
      }
      const cols = lines[lines.length - 1].trim().split(/\s+/);
      const [, totalKb, usedKb, availKb, pct] = cols;
      resolve({
        path,
        total_kb: Number(totalKb),
        used_kb: Number(usedKb),
        available_kb: Number(availKb),
        used_pct: Number(String(pct || "").replace("%", "")) || null,
      });
    });
  });
}

const handlers = {
  initialize: (req) => ({
    protocolVersion: req.params?.protocolVersion || PROTOCOL_VERSION,
    capabilities: { tools: {} },
    serverInfo: { name: "sys-info", version: "1.0.0" },
  }),
  "tools/list": () => ({ tools: [TOOL_SYS_INFO, TOOL_DISK_USAGE] }),
  "tools/call": async (req) => {
    const name = req.params?.name;
    try {
      if (name === "sys_info") {
        return textResult(JSON.stringify(sysInfo(), null, 2));
      }
      if (name === "disk_usage") {
        const p = req.params?.arguments?.path;
        if (typeof p !== "string" || !p.startsWith("/")) {
          return { ...textResult("error: `path` must be an absolute path"), isError: true };
        }
        return textResult(JSON.stringify(await diskUsage(p), null, 2));
      }
      return { ...textResult(`error: unknown tool ${name}`), isError: true };
    } catch (e) {
      return { ...textResult(`error: ${e.message}`), isError: true };
    }
  },
  ping: () => ({}),
};

const readline = require("readline");
const rl = readline.createInterface({ input: process.stdin });
rl.on("line", (line) => {
  line = line.trim();
  if (!line) return;
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return;
  }
  if (msg.id === undefined || msg.id === null) return;
  const handler = handlers[msg.method];
  Promise.resolve(handler ? handler(msg) : { error: { code: -32601, message: `unknown method: ${msg.method}` } })
    .then((result) => {
      if (process.stdout.writable) process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result }) + "\n");
    })
    .catch((e) => {
      if (process.stdout.writable)
        process.stdout.write(
          JSON.stringify({ jsonrpc: "2.0", id: msg.id, error: { code: -32603, message: String(e && e.message || e) } }) + "\n"
        );
    });
});
