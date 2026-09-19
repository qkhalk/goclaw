#!/usr/bin/env node
/**
 * media-probe — zero-dependency MCP stdio server.
 *
 * Exposes one tool, `probe`, that runs ffprobe on a media file and returns
 * its format/stream metadata as JSON. Speaks MCP over stdin/stdout with
 * newline-delimited JSON-RPC 2.0 (no SDK dependency on purpose: nothing to
 * install, no supply chain, ~60MB RSS).
 */
"use strict";

const { spawn } = require("child_process");
const readline = require("readline");

const PROTOCOL_VERSION = "2025-06-18";
const FFPROBE = process.env.FFPROBE_PATH || "ffprobe";

const TOOL_PROBE = {
  name: "probe",
  description:
    "Probe a media file (image/video/audio) with ffprobe and return duration, codec, resolution, bitrate and stream layout as JSON.",
  inputSchema: {
    type: "object",
    properties: {
      path: { type: "string", description: "Absolute path to the media file." },
    },
    required: ["path"],
  },
};

/** Run ffprobe -print_format json and resolve with its stdout. */
function probe(path) {
  return new Promise((resolve, reject) => {
    const args = [
      "-v", "error",
      "-print_format", "json",
      "-show_format",
      "-show_streams",
      "--",
      path,
    ];
    const child = spawn(FFPROBE, args, { stdio: ["ignore", "pipe", "pipe"] });
    let out = "";
    let err = "";
    child.stdout.on("data", (d) => (out += d));
    child.stderr.on("data", (d) => (err += d));
    child.on("error", (e) => reject(new Error(`ffprobe not runnable: ${e.message}`)));
    child.on("close", (code) => {
      if (code !== 0) {
        reject(new Error(`ffprobe exited ${code}: ${err.trim().slice(-400)}`));
        return;
      }
      try {
        resolve(JSON.parse(out));
      } catch (e) {
        reject(new Error(`ffprobe output not JSON: ${e.message}`));
      }
    });
  });
}

/** Content blocks for a tools/call result. */
function textResult(text) {
  return { content: [{ type: "text", text }] };
}

const handlers = {
  initialize: (req) => ({
    protocolVersion: req.params?.protocolVersion || PROTOCOL_VERSION,
    capabilities: { tools: {} },
    serverInfo: { name: "media-probe", version: "1.0.0" },
  }),
  "tools/list": () => ({ tools: [TOOL_PROBE] }),
  "tools/call": async (req) => {
    const path = req.params?.arguments?.path;
    if (typeof path !== "string" || path.length === 0) {
      return { ...textResult("error: `path` (string) is required"), isError: true };
    }
    try {
      const info = await probe(path);
      return textResult(JSON.stringify(info, null, 2));
    } catch (e) {
      return { ...textResult(`error: ${e.message}`), isError: true };
    }
  },
  ping: () => ({}),
};

const rl = readline.createInterface({ input: process.stdin });
rl.on("line", (line) => {
  line = line.trim();
  if (!line) return;
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return; // not JSON-RPC — ignore
  }
  if (msg.id === undefined || msg.id === null) return; // notification (incl. initialized)
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
