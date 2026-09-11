---
name: fuzz
description: Use when asked to discover hidden or forgotten endpoints, files, and parameters on a web application the requester owns or is authorized to test before production — content discovery with ffuf (directories, staging files, admin panels, debug routes), with rate limiting and response filtering, producing a cleanup-oriented findings list. Not for attacking third-party systems and not for input-fuzzing that could corrupt data.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target
  - wordlist
outputs:
  - discovery_report
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - authorization_confirmed
  - scope_locked
  - discovery_report_generated
deps:
  - system:ffuf
  - system:curl
---

# Fuzz (Content Discovery)

Find what a web application exposes that it shouldn't before it goes to
production: leftover staging files, debug routes, admin panels, backup
archives, undocumented API endpoints. Discovery and cleanup-oriented — not
exploitation.

## Authorization gate (mandatory, first)

Content discovery sends thousands of requests and looks like an attack in
access logs. Before ANY request:

1. **Ownership or written authorization** for the target. If the requester
   does not demonstrably control the host (or hold permission), refuse.
2. **Scope lock.** Exact in-scope hostnames and URL prefixes; anything not
   listed is out of scope. Note exclusions (e.g. do not touch `/admin`
   beyond confirming it exists).
3. **Rate limits agreed up front.** Default `-rate 50` requests/second or
   lower against shared/staging systems; confirm an acceptable rate for
   production systems and prefer off-peak windows.
4. **Stop conditions** (abort immediately): error rate spikes from the
   target, WAF/IDS throttling or bans, target slows visibly, or anyone from
   the requester side says stop.

## Method

1. **Baseline.** Fetch a known-good path and a known-bad path and record
   their size/word/line counts — every filter below hangs off these:
   ```sh
   curl -sS -o /dev/null -w '%{size_download} %{http_code}\n' https://<host>/
   curl -sS -o /dev/null -w '%{size_download} %{http_code}\n' https://<host>/definitely-not-here-9f3c
   ```
2. **Directory/file discovery** with the built-in or supplied wordlist:
   ```sh
   ffuf -w /usr/share/wordlists/dirb/common.txt -u https://<host>/FUZZ \
        -rate 50 -t 10 -mc 200,201,301,302,401,403 \
        -fs <baseline-size> -o ffuf-roots.json
   ```
   Filter noise with `-fs/-fw/-fl` (size/words/lines from the baseline), not
   by dropping status codes — 401/403 hits are findings, not noise.
3. **Extension sweep** on interesting roots only (never the whole
   wordlist × every extension): append common extensions for the stack
   (`.bak .old .sql .zip .env .json .yml .tgz`) to the paths worth checking.
4. **Virtual host probe** (only if the requester confirms wildcard DNS):
   `ffuf -w subdomains.txt -u https://<host>/ -H 'Host: FUZZ.<host>' -fs <baseline-size> -rate 50`
5. **Verify by hand.** For every hit, one `curl -sSII` to confirm it is real
   and capture the response. Automation hits are leads; verified hits are
   findings.

## Abort criteria

Stop and report immediately when the target starts erroring or slowing
(you may be affecting real users), when bans/throttling appear, or when a hit
clearly contains live user data — report it, do not download or explore it.

## Report

Write `discovery-report.md`:

- Scope recap (hosts, wordlists, rate, window, authorization reference).
- Findings table: URL × status × size × what it is × risk (High: exposed
  credentials/backups/admin with no auth; Medium: debug endpoints, verbose
  errors; Low: missing-rate-limit hints, directory listing).
- For each finding: one-line remediation (remove, auth-gate, or stop
  serving) — this is a pre-production cleanup list.
- Reproduction: exact ffuf commands and wordlists used.

## Rules

- Never fuzz outside the written scope; wildcard-DNS vhost probing needs
  explicit confirmation.
- Rate limits are hard: if `-rate` was agreed, it is not negotiable
  mid-run.
- Read-only requests only: no POST/PUT/DELETE fuzzing, no parameter payloads
  that could mutate state.
- Verify every automated hit manually before it enters the report.
