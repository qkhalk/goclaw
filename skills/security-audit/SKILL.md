---
name: security-audit
description: Use when asked to assess the security of a web application, API, or host that the requester owns or is explicitly authorized to test before production launch. Runs a structured audit (recon, TLS, headers, known-vulnerability scans, scoped injection checks, auth/session review) and produces a severity-ranked findings report with remediations. Not for probing third-party systems.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target
  - scope
outputs:
  - findings_report
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - authorization_confirmed
  - scope_locked
  - report_generated
deps:
  - system:nmap
  - system:nikto
  - pip:sqlmap
  - system:testssl.sh
  - system:curl
---

# Security Audit

Authorized security assessment of a web application, API, or host before it
goes to production. The output is an evidence-backed findings report ranked by
severity, with concrete remediation for every finding.

## Authorization gate (mandatory, first)

Before running ANY scan, confirm ALL of the following. If any is missing,
STOP and ask the requester instead of scanning:

1. **Ownership or written authorization.** The target belongs to the requester,
   or they can point to written permission (engagement letter, ticket, email)
   from the system owner. Generic claims like "it's fine, just scan it" are
   not authorization for systems you cannot verify they own.
2. **Scope lock.** Record the exact in-scope hostnames, IPs, ports, URL paths,
   and any explicit exclusions. Every command you run later must stay inside
   this list. Default-deny: anything not listed is out of scope.
3. **Test environment preference.** If a staging environment exists, audit it
   first. Auditing production is allowed only when the requester confirms the
   system can tolerate scan traffic.
4. **Intensity ceiling.** Unauthenticated, rate-limited scanning only unless
   the requester explicitly provides test credentials and asks for
   authenticated testing. Never run destructive checks (drop/delete payloads,
   forced logout of real users) against shared environments.

If the target resolves to infrastructure the requester clearly does not
control (shared hosting they only rent a page on, third-party SaaS, cloud
services they do not own), decline that part of the scope and explain why.

## Method

Work through the phases in order. Collect evidence (command + trimmed output)
for every finding. Skip phases whose tools are missing and note it in the
report — each phase has a curl/openssl fallback.

### Phase 1 — Recon

Map the attack surface before touching it:

```sh
# Service and version scan (rate-limited)
nmap -sV -Pn --max-rate 100 -p- <host>          # full port sweep, slow
nmap -sV -Pn --max-rate 100 --top-ports 1000 <host>  # quick pass

# Technology fingerprint + headers
curl -sSI https://<host>/                       # note Server, X-Powered-By
curl -sS https://<host>/robots.txt
```

Record: open ports and services, framework/CMS versions, exposed admin paths,
interesting headers. Check that only expected ports are open — anything else
is a finding.

### Phase 2 — TLS

```sh
testssl.sh --quiet https://<host>/              # full grade
# fallback:
openssl s_client -connect <host>:443 -tls1_2 </dev/null 2>&1 | head -20
```

Findings: expired/near-expiry certs, SSLv3/TLS1.0/1.1 enabled, weak ciphers
(3DES/RC4), missing HSTS, no OCSP stapling, certificate hostname mismatch.

### Phase 3 — Web layer

```sh
# Known-vulnerability + misconfiguration scan
nikto -h https://<host>/ -Tuning 123bde -timeout 10

# Security headers baseline (curl fallback)
curl -sSI https://<host>/ | grep -iE 'strict-transport|x-frame|x-content-type|content-security|referrer-policy|permissions-policy'
```

Expected headers: HSTS, X-Frame-Options/CSP frame-ancestors,
X-Content-Type-Options, Content-Security-Policy, Referrer-Policy. Missing
ones are low/medium findings depending on the app type. Also check: directory
listing enabled, backup/source files (`.git/`, `.env`, `*.bak`) reachable,
default credentials pages exposed, verbose error pages leaking stack traces.

### Phase 4 — Scoped injection checks

Only against in-scope URLs with a clearly marked test parameter. Never run
these against pages that write to shared state.

```sh
# SQL injection, conservative depth, read-only techniques
sqlmap -u "https://<host>/page?id=1" --batch --level=1 --risk=1 \
  --technique=BE --skip-urlencode --threads=1
```

Stop conditions: any confirmed injection → record evidence, do NOT attempt
data extraction beyond `--dbs`/current user on explicit requester approval.
For XSS, probe reflected input manually with benign markers (`<b>xss</b>`,
`"xssprobe`) and record reflection context; do not attempt session theft or
payloads that persist for other users.

### Phase 5 — Auth & session review

Read the application's auth behavior via curl: cookie flags (`Secure`,
`HttpOnly`, `SameSite`), session token rotation on login, login rate limiting
and lockout, password/2FA policy on the login page, JWT `alg` if tokens are
used (`alg=none` or key-confusion issues), IDOR spot-checks on 2 test
accounts if provided (access object of user A with session of user B).

### Phase 6 — Report

Produce `security-audit-report.md` in the workspace:

- Executive summary (3–5 sentences, overall posture).
- Scope + authorization note (who authorized, when, what was in/out).
- Findings table: ID | Severity (Critical/High/Medium/Low/Info) | Title |
  Evidence (command + trimmed output) | Affected component | Remediation.
- Method appendix: exact commands per phase, tools that were missing.
- "Not tested" section — phases skipped and why (tool missing, out of scope).

Every finding MUST cite evidence you actually captured. No speculative
findings without reproduction.

## Rules

- One phase at a time; re-check scope before each new command template.
- Rate-limit every network tool (`--max-rate`, `--threads 1`, `-c 1`).
- Never modify or delete data. If a check could write (POST forms), use a
  scratch account or skip.
- If any tool errors with "command not found", use the fallback and note it.
- Escalation: stop and report immediately if you find evidence of an ACTIVE
  compromise (unexpected shells, unknown admin users) — do not "investigate"
  by pivoting.
