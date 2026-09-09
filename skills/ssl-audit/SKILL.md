---
name: ssl-audit
description: Use when asked to check the TLS/SSL health of a hostname before launch — certificate expiry and chain, protocol versions, cipher strength, HSTS, OCSP stapling. Produces a pass/fail table with remediations. Lighter and faster than a full security-audit when only the TLS layer is in question.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target
outputs:
  - tls_report
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - report_generated
deps:
  - system:testssl.sh
  - system:openssl
---

# TLS/SSL Audit

Focused TLS health check for a hostname the requester controls: certificate
chain and expiry, protocol and cipher hygiene, and transport hardening
headers. This is read-only — it never sends attack traffic, so it is safe on
production endpoints.

## Scope check (first)

Confirm the hostname belongs to the requester or they are authorized to
assess it. TLS handshakes are passive, but there is no reason to probe
third-party systems uninvited — if asked to audit a hostname the requester
clearly does not own, ask for the relationship first.

## Checks

```sh
# Full grade (preferred)
testssl.sh --quiet --color 0 https://<host>/

# --- openssl fallbacks per check ---
# Certificate: expiry, SANs, issuer, chain completeness
openssl s_client -connect <host>:443 -servername <host> </dev/null 2>/dev/null | openssl x509 -noout -dates -subject -issuer -ext subjectAltName

# Chain verification (each hop)
openssl s_client -connect <host>:443 -servername <host> -showcerts </dev/null 2>/dev/null | grep -E '^ [0-9] s:|^   i:'

# Protocol floor: each of these must FAIL (handshake rejected)
openssl s_client -connect <host>:443 -tls1   </dev/null 2>&1 | grep -c 'protocol version'   # expect >0
openssl s_client -connect <host>:443 -tls1_1 </dev/null 2>&1 | grep -c 'protocol version'   # expect >0
# TLS 1.2 and 1.3 must SUCCEED
openssl s_client -connect <host>:443 -tls1_2 </dev/null 2>&1 | grep 'Protocol'

# OCSP stapling
openssl s_client -connect <host>:443 -status </dev/null 2>&1 | grep -A2 'OCSP response'

# Security headers
curl -sSI https://<host>/ | grep -iE 'strict-transport-security'
```

## Verdict table

Every row is PASS / WARN / FAIL with the observed value:

| Check | Pass criteria |
|-------|---------------|
| Cert validity | ≥ 30 days remaining; ≤ 90 days preferred (rotation headroom) |
| Hostname match | SAN covers the exact hostname (no wildcards wider than needed) |
| Chain | Complete, no self-signed intermediates, no SHA-1 |
| Protocols | TLS 1.2+ only; SSLv3/TLS1.0/TLS1.1 and cleartext :80 redirects present |
| Ciphers | No RC4/3DES/NULL/EXPORT; forward secrecy on ≥ TLS 1.2 |
| HSTS | Present with `max-age ≥ 15552000`; `includeSubDomains` noted if applicable |
| OCSP stapling | Responds (WARN if absent) |
| Session resumption | Works (WARN only) |

## Report

Write `ssl-audit-report.md`: verdict table with evidence per row, one-line
remediation for each WARN/FAIL (typical: "renew via ACME with auto-reload",
"disable TLS 1.0/1.1 in the listener/CDN config", "add HSTS header at the
edge"), and the reproduction commands. If `testssl.sh` is missing, say so and
rely on the openssl fallbacks — mark protocol/cipher rows as best-effort.

## Rules

- Read-only: handshake, read, disconnect. No client certs against other
  people's endpoints, no TLS-crushing flood tools.
- Report the observed value, not the tool's logo grade — a "B" from a tool
  means nothing without the failing rows.
