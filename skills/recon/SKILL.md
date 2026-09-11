---
name: recon
description: Use when asked to map the exposed surface of a system the requester owns or is authorized to test before a production launch or security audit — host discovery, open ports, service and version identification, and a plain-language attack-surface summary. Discovery only: no exploitation, no vulnerability scanning (hand off to security-audit for that). Only for targets the requester owns or is explicitly authorized to test.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target
  - scope
outputs:
  - surface_map
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - authorization_confirmed
  - scope_locked
  - surface_map_generated
deps:
  - system:nmap
  - system:curl
---

# Recon (Service Discovery)

Map what a system exposes to the network before it goes to production — or
before a deeper security audit: which hosts answer, which ports are open, what
is actually listening on them, and what that surface implies. Discovery only.

## Authorization gate (mandatory, first)

Port scanning is legitimate only against systems you are allowed to scan, and
many networks log and alert on it. Before ANY probe:

1. **Ownership or written authorization** for every target IP/hostname. If the
   requester cannot demonstrate they control the target (or hold permission
   from whoever does), do not scan it — refuse that part of the scope.
2. **Scope lock.** Write down the exact in-scope hosts/CIDRs and ports. Every
   command must stay inside this list. Default-deny: anything not listed is
   out of scope.
3. **Scan window.** Confirm when it is OK to scan. Staging first if one
   exists; production only with explicit confirmation that scan traffic is
   acceptable, ideally off-peak.
4. **Intensity ceiling.** Default to a rate-limited, non-intrusive profile.
   No exploitation, no brute force, no DoS-adjacent flags (`-T5` against
   shared/production systems, NSE intrusive scripts).

## Method

1. **Confirm reachability** of each target before scanning it:
   `curl -sS -o /dev/null -w '%{http_code} %{time_total}\n' https://<host>/`
   — record whether the host answers at all and on which scheme.
2. **Port discovery** (top ports first, full range only if requested):
   `nmap -Pn --top-ports 1000 -T3 <host>`
   Record open/filtered per port. Skip ICMP ping hosts (`-Pn`) so firewalls
   that drop ping don't hide open ports.
3. **Service + version identification** on the open ports only:
   `nmap -Pn -sV -sV --version-intensity 2 -p <open-ports> <host>`
   Add `--version-light` behavior by keeping intensity low — fingerprint
   without hammering.
4. **HTTP surface check** on discovered web ports (read-only):
   `curl -sSI https://<host>:<port>/` and the security headers line. Note
   server banners, redirects, and whether admin paths answer with 401/403 or
   200 (do not enumerate paths — that is fuzz/security-audit territory).
5. **Summarize the surface** in one table: host → port → service → version →
   exposure note (public-facing? banner leaks version? TLS or plaintext?).

## Abort criteria

Stop and report immediately when: a target stops responding mid-scan (you may
be affecting it — back off and tell the requester), an IDS/WAF starts
throttling or blocking you, or you find evidence the target is not what the
requester said it was (different owner, shared hosting).

## Report

Write `recon-report.md`:

- Scope recap (hosts, ports, window, authorization reference).
- Surface table: host × port × service × version × exposure note.
- Surprises: unexpected open ports, outdated-seeming versions, plaintext
  services on public interfaces, management interfaces reachable from outside.
- Recommended next steps: which findings to feed into `security-audit`, which
  ports to close or firewall before launch.
- Reproduction: exact nmap/curl commands per host.

## Rules

- Never scan outside the written scope, even if a neighboring host looks like
  part of the same system.
- Keep scan rates polite (`-T3` default); the goal is a map, not pressure.
- Evidence over memory: every claim in the report needs the command that
  produced it.
- This skill stops at discovery. Hand vulnerability findings and exploitation
  checks to `security-audit` with the requester's confirmation.
