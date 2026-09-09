---
name: netstress
description: Use when asked to test Layer-4 resilience of owned infrastructure before production — TCP/UDP throughput ceilings, packets-per-second, connection-rate limits, and saturation behavior under sustained transport-layer pressure (iperf3, hping3 rate-limited). Measures how the network path and server stack behave at capacity. Only for hosts the requester owns or is explicitly authorized to stress; never for third-party systems.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target
  - protocol
outputs:
  - resilience_report
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - authorization_confirmed
  - test_window_confirmed
  - resilience_report_generated
deps:
  - system:iperf3
  - system:hping3
  - system:curl
---

# Network Stress Test (Layer 4)

Transport-layer capacity and resilience testing for infrastructure you
control, before it faces production traffic: how many packets, how much
bandwidth, how fast a connection rate the path and the server stack can
absorb before behavior degrades.

This is deliberately a **capacity measurement** discipline, not an attack
simulation: everything is rate-limited, monitored, and reversible.

## Authorization gate (mandatory, first)

Layer-4 stress affects every service on the box, not just one app. Confirm
ALL of:

1. **Both endpoints owned.** The generator side AND the target side must be
   requester-controlled. Testing toward any third-party host (ISP gateway,
   public service, shared load balancer you rent a slice of) is out of scope
   — refuse it. If you need an iperf3 server, start one on the requester's
   own box (`iperf3 -s`) and test between the requester's own machines.
2. **Scheduled window.** The requester names a start time and duration;
   anyone sharing the environment is informed. Default window: 15 minutes.
3. **Neighbor blast radius.** If the target hosts other services or shares a
   link with other tenants, flag it and get explicit acknowledgment. If it is
   a shared environment without isolation, restrict tests to loopback or a
   dedicated test pair.
4. **Abort conditions** — stop generating immediately when:
   - the target's health check fails (from another terminal:
     `while true; do curl -sm2 -o /dev/null -w '%{http_code}\n' http://<target-health>; sleep 2; done`),
   - packet loss on OTHER services on the box is observed,
   - the window expires,
   - anyone says stop.

## Measurements

Run each class separately, lowest intensity first. Record the ceiling for
each. All hping3 invocations MUST include `--rate` (a hard cap) — never run
unthrottled packet floods.

### 1. TCP throughput ceiling (iperf3)

```sh
# On the requester's target host:
iperf3 -s
# From the generator:
iperf3 -c <target-ip> -t 30 -P 4        # 4 parallel streams, 30s
iperf3 -c <target-ip> -t 30 -R          # reverse direction
iperf3 -c <target-ip> -t 30 -u -b 500M  # UDP at bounded bitrate
```

Record: achieved Mbps/Gbps per direction, retransmits (TCP), jitter + loss
(UDP). The UDP number that first shows loss is the path's practical pps/bit
ceiling.

### 2. Connection-rate ceiling (hping3, rate-capped)

Measures how many new TCP handshakes per second the target stack accepts
before SYN backlog drops:

```sh
# SYN rate test — ALWAYS rate-capped, short duration
hping3 -S -p 443 --rate 200 -c 2000 <target-ip>
hping3 -S -p 443 --rate 500 -c 5000 <target-ip>
# (fallback if hping3 missing: nmap's nping --tcp -c 2000 --rate 200 -p 443 <ip>)
```

Step the rate up slowly (200 → 500 → 1000…). The ceiling is the rate where
`ss -s` on the target shows SYN-RECV growth or handshakes start timing out.

### 3. Concurrent connection ceiling

```sh
# hold N idle keepalive connections, step N up
hey -z 60s -c 500 -o csv https://<host>/healthz >/dev/null
ss -tn state established | wc -l         # on target, sample during the hold
```

Watch the target's FD limits (`ulimit -n`, systemd `LimitNOFILE`) and
conntrack table (`conntrack -C` or `/proc/sys/net/netfilter/nf_conntrack_count`
vs `_max`). The first resource to hit its limit names the ceiling.

### 4. Saturation behavior

At ~80% of the slowest ceiling from steps 1–3, hold load 5 minutes and
observe: does latency of a normal request degrade gracefully (measure with
`curl -w '%{time_total}'` in a loop) or cliff? Are errors clean (refused,
RST) rather than hangs? Graceful degradation with clean errors is the
pre-production pass criterion.

## Report

Write `netstress-report.md`:

- Ceiling table: TCP throughput (both directions), UDP loss onset, SYN rate,
  concurrent connections — each with the limiting resource named.
- Saturation behavior paragraph (what degrades first, how it fails).
- Target-side config ceilings observed: `ulimit -n`, `somaxconn`,
  `nf_conntrack_max`, backlog queue sizes — with the values found.
- Reproduction commands and the window used.
- Recommendations: the specific sysctl/limit changes or LB/autoscaling
  triggers that would raise each ceiling, in impact order.

## Rules

- No distributed generators: one generator host at a time, from the
  requester's own infra.
- Every rate capped (`--rate`, `-b`, `-c`, `-z`/`-t` duration) — no open-ended
  commands.
- Watch the health check for the whole window; abort rules are mechanical.
- If the box is shared and isolation is unclear, downgrade to loopback
  (`iperf3 -c 127.0.0.1`) tests and say so in the report.
