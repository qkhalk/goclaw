---
name: dns-audit
description: Use when asked to check the DNS health of a domain the requester owns or manages before launch — SPF, DKIM, and DMARC email-safety records, resolver propagation, common misconfigurations (multiple SPF records, DMARC p=none, missing DKIM selectors), and a plain-language hardening checklist. Read-only queries against authoritative/resolving DNS; only for domains the requester controls.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - domain
outputs:
  - dns_report
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - ownership_confirmed
  - dns_report_generated
deps:
  - system:dig
  - system:curl
---

# DNS Audit (Email Safety & Zone Health)

Read-only DNS health check for a domain you control, before launch: whether
mail from your domain can be spoofed (SPF/DKIM/DMARC), whether records
resolve consistently, and what to harden. Zero impact on the target — this
only issues ordinary DNS queries.

## Ownership gate (mandatory, first)

1. The requester should control the domain (their own company/product domain,
   or a client domain they manage). Auditing unrelated third-party domains is
   out of scope even though queries are harmless — findings belong to the
   owner.
2. Record the exact domain(s) and, if relevant, subdomains in scope.

## Checks

Run each with a public resolver and record raw output:

```sh
# Registration + nameservers
dig +short SOA <domain>
dig +short NS <domain>

# Email safety (the core of this audit)
dig +short TXT <domain>                 # SPF lives here
dig +short TXT _dmarc.<domain>          # DMARC policy
dig +short TXT default._domainkey.<domain>   # DKIM (also try selector names
dig +short TXT google._domainkey.<domain>    # the requester actually uses:
dig +short TXT selector1._domainkey.<domain> # ask if the first three miss)

# Mail routing + WWW health
dig +short MX <domain>
dig +short A www.<domain>
dig +short AAAA www.<domain>
```

Compare answers against the classification below.

## Findings classification

- **SPF missing or ends with `+all`/`?all`** → High: anyone can spoof mail
  from the domain. Fix: publish an SPF with `-all`.
- **Multiple SPF records** → High: RFC 4408 says two TXT SPF records make
  the record invalid — merge into one.
- **SPF over 10 DNS lookups** → Medium: lookups beyond 10 are ignored;
  flatten includes or use SPF macros.
- **DMARC missing** → High: no spoofing protection signal for receivers.
  Fix: publish `_dmarc` starting at `p=quarantine; rua=mailto:...`.
- **DMARC `p=none` without `rua=`** → Medium: policy exists but is
  monitor-only with no reports address — either add `rua` and monitor, or
  move to `p=quarantine`.
- **DKIM missing** → Medium (if the domain sends mail): sign outbound mail
  and publish the selector.
- **MX points to a host not covered by SPF include** → Medium: verify the
  include chain actually authorizes the MX that receives mail.
- **NS single point / missing AAAA / SOA mismatch** → Low/resiliency notes.

## Report

Write `dns-report.md`:

- Domain, resolver used, timestamp.
- Findings table: record × current value × classification × concrete fix
  (the exact record line to publish).
- Email-safety verdict line: "spoofable / partially protected / protected".
- Hardening checklist ordered by impact.
- Reproduction: the dig commands and their outputs (trimmed).

## Rules

- Read-only: this skill never edits zones or asks registrars for changes —
  the requester applies fixes and re-runs the audit to verify.
- Treat every finding as belonging to the owner: report, don't probe deeper
  into services the records reveal.
- If the domain uses a provider (Google Workspace, M365), check the
  provider's documented selector names before declaring DKIM missing.
