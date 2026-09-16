---
name: mcp-builder
description: >-
  Build MCP (Model Context Protocol) servers: tool, resource, and prompt primitives; stdio
  vs HTTP transports; schema design; authentication; testing; and packaging for
  distribution. Use when creating or reviewing an MCP server that exposes capabilities to
  LLM agents. Keywords: MCP, Model Context Protocol, tools, stdio, HTTP. Dùng khi xây dựng
  MCP server, viết tool cho AI agent, tích hợp giao thức MCP.
license: MIT
version: 1
---

# MCP Builder

Design and ship Model Context Protocol servers: choose primitives and transport, design tool schemas the model can use correctly, secure and test the server, and package it for distribution.

## When to use
- Exposing a system's capabilities to LLM agents via MCP
- Reviewing an existing MCP server for schema, safety, or transport issues
- Deciding between stdio and streamable HTTP transports
- Packaging and distributing an MCP server (npm, pip, binary)

## When NOT to use
- MCP client/bridge behavior inside goclaw → the repo's `internal/mcp` conventions
- Agent design on Google's stack → `goclaw-kit`
- General in-process tool design for goclaw itself → follow `internal/tools` patterns

## Workflow
1. Pick primitives deliberately: tools for actions the model chooses (side effects allowed), resources for read-only context addressed by URI, prompts for reusable message templates. Most servers need 3-8 good tools, not 30 thin ones.
2. Design each tool around the model reading it: a tight imperative description ("Call after X; returns Y"), JSON Schema with required fields, enums over free strings, and flat argument shapes — deep nesting raises error rates.
3. Return model-friendly results: concise text or structured content the model can act on; put machine details in metadata; on failure return a typed, actionable error message, never a raw stack trace.
4. Choose transport: stdio for local single-user tools launched by the client; streamable HTTP for shared/remote servers needing auth and multi-client access.
5. Secure it: for HTTP, authenticate requests and authorize per tool; for stdio, rely on process isolation but still validate inputs; treat every tool argument as untrusted input.
6. Make side-effecting tools safe: dry-run flags, explicit confirmation arguments for risky operations, and idempotent handlers so client retries do not double-apply.
7. Test at the protocol level: run the server under the official inspector and a scripted client; cover success, validation failure, and tool error paths; assert the declared schema matches what the code returns.
8. Package for distribution: publish as an npm/pip package or single binary; document install, config (env var names, secrets out of band), and a minimal client config snippet; version the tool schemas.

## Output
- A working MCP server (tools/resources/prompts with schemas), protocol-level tests, and a README with install, config, and safety notes for clients.

## Routing
- Consuming MCP servers from goclaw → repo `internal/mcp` bridge conventions
- Building agents on Google ADK with MCP tools → `goclaw-kit`
- Security review of exposed tools → `security-audit`

## Guardrails
- Validate every tool argument against the declared schema before executing.
- No secrets in tool descriptions, schemas, or error messages; read them from the environment.
- Prefer read-only and dry-run defaults; make destructive actions explicit and confirmable.
- Keep tool count and description length small — the model's context pays for every tool.
