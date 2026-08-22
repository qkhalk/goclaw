# GoClaw — Remaining Work Completion Plan
## Durable Agent Runtime + AgentKit Native Runtime + 2026 Advanced Agents

> Base repo: https://github.com/qkhalk/goclaw
> AgentKit: https://agentkit.best/docs

## 0. Mục tiêu

Hoàn thiện các phần còn thiếu sau các cải tiến hiện tại, tập trung vào 3 vấn đề gốc:

1. `Too many requests` không được làm chết run hoặc tạo retry storm.
2. `Something went wrong` không được xuất hiện khi agent vẫn đang chạy.
3. Model yếu có thể trả output lỗi, gọi tool sai hoặc báo `done` sớm nhưng runtime phải có khả năng repair, recover, verify và tiếp tục.

Đích đến:

```text
LLM proposes
→ Agent executes
→ Runtime observes
→ Recovery repairs
→ Verifier decides
→ Checkpoint persists
→ Event Store records
→ Workflow continues/completes
```

Không lấy output của LLM, WebSocket hay HTTP request làm source of truth. Source of truth là `WorkflowRun State + Events + Checkpoints + Artifacts + Verification`.

---

# 1. Đã có và không làm lại

```text
[✓] reliability package
[✓] canonical error taxonomy
[✓] rate-limit coordinator
[✓] circuit breaker
[✓] provider/model health scoring
[✓] metrics/SLO foundation
[✓] /gc: parser + registry + executor
[✓] /gc:plan /gc:fix /gc:cook /gc:review
[✓] skill metadata + built-in skills
[✓] completion verifier foundation
[✓] JSON repair
[✓] tool-loop detection
[✓] checkpoint foundation
[✓] hibernation foundation
[✓] mission foundation
[✓] event sequencing
```

Roadmap này tập trung **integration + runtime behavior**, không tạo thêm nhiều primitive rời rạc.

---

# 2. Priority

## P0 — Reliability / correctness

```text
[x] Durable Run State Machine
[x] Real Recovery Engine
[x] Completion Verifier as terminal gate
[x] Stream disconnect recovery
[x] Provider/model fallback integration
[x] Shared 429 cooldown integration
[x] Adaptive retry
[x] Empty-output recovery
[x] Malformed tool-call recovery
[x] Premature-completion recovery
[x] Stalled-run watchdog
[x] Reconciliation worker
```

## P1 — AgentKit native runtime

```text
[ ] Native Skill Registry
[ ] Skill Executor
[ ] Workflow DAG
[ ] Hook engine
[ ] Artifact engine
[x] Quality gates
[x] Kit loader/versioning
[x] Skill dependency resolver
[x] Runtime-enforced tool permissions
```

## P2 — Agent UX / control plane

```text
[ ] /gc:architect
[ ] /gc:research
[ ] /gc:debug
[ ] /gc:test
[ ] /gc:security
[ ] /gc:refactor
[ ] /gc:docs
[ ] /gc:optimize
[ ] /gc:mission
[x] /gc:status
[x] /gc:runs
[x] /gc:doctor
[ ] /gc:resume
[ ] /gc:retry
[ ] /gc:pause
[ ] /gc:cancel
[ ] /gc:fork
[x] /gc:approve
[ ] /gc:explain
```

## P3 — Model intelligence

```text
[ ] Model capability profiles
[ ] Reliability-aware model router
[ ] Provider pool
[ ] Cost-aware routing
[ ] Strategy router
[ ] Smart retry
[ ] Experience store
[ ] A/B evaluation
```

## P4 — Multi-agent

```text
[ ] Structured handoff
[ ] Dynamic team formation
[ ] Competitive agents
[ ] Reviewer/jury
[ ] Consensus
[ ] Budget control
```

## P5 — 2026 advanced

```text
[ ] Mission 2.0
[ ] Hibernation 2.0
[ ] Agent time travel / fork
[ ] Predictive failure detection
[ ] Simulation mode
[ ] Chaos testing
[ ] Project intelligence index
[ ] Self-improvement loop
```

---

# 3. Phase 1 — Durable Agent State Machine

## Target states

```text
QUEUED
RUNNING
THINKING
WAITING_PROVIDER
WAITING_TOOL
RECOVERING
VERIFYING
PAUSED
CANCELLED
COMPLETED
FAILED
```

Implement:

```text
internal/agent/runtime/
  state.go
  machine.go
  transition.go
  run.go
  reconcile.go
```

Rules:

- invalid transitions are rejected;
- state is durable;
- process restart does not lose run state;
- transport disconnect never automatically means `FAILED`.

Acceptance:

```text
[ ] state persists across restart
[ ] invalid transition tests
[ ] WebSocket disconnect keeps run alive
[ ] worker crash can resume from checkpoint
```

---

# 4. Phase 2 — Durable Event Store

Canonical event:

```json
{
  "run_id": "...",
  "seq": 120,
  "type": "tool.completed",
  "timestamp": "...",
  "payload": {}
}
```

Events:

```text
run.created
run.started
llm.started
llm.completed
llm.failed
tool.started
tool.completed
tool.failed
checkpoint.created
recovery.started
recovery.completed
verification.started
verification.failed
verification.passed
run.paused
run.resumed
run.completed
run.failed
```

Requirements:

```text
monotonic sequence
idempotent append
replay
retention
correlation
```

Reconnect flow:

```text
client last_seq=N
→ server replay N+1...
```

---

# 5. Phase 3 — Checkpoint 2.0

Checkpoint must persist:

```text
run state
workflow position
current step
model/provider
context snapshot
retry counters
budget
tool state
artifact refs
```

Checkpoint points:

```text
after planning
after tool call
after patch
after test
before side effect
after recovery
before completion
```

Commands:

```text
/gc:resume <run-id>
/gc:pause <run-id>
```

Resume must continue from the next legal transition, never replay the whole task by default.

---

# 6. Phase 4 — Real Recovery Engine

Architecture:

```text
Failure
→ Error Classifier
→ Recovery Policy
→ retry / wait / repair / fallback / strategy-switch
→ checkpoint
→ continue
→ verify
```

Error classes:

```text
PROVIDER_RATE_LIMIT
PROVIDER_UNAVAILABLE
PROVIDER_TIMEOUT
MODEL_EMPTY
MODEL_MALFORMED
MODEL_PREMATURE_COMPLETION
MODEL_LOOPING
TOOL_TIMEOUT
TOOL_INVALID_ARGS
TOOL_TRANSIENT
CONTEXT_OVERFLOW
RESOURCE_LIMIT
POLICY_DENIED
VERIFICATION_FAILED
```

Each class defines:

```text
retryable
max_attempts
backoff
fallback
checkpoint
strategy_switch
```

---

# 7. Phase 5 — Fix `429 Too Many Requests`

Required flow:

```text
429
→ parse Retry-After
→ shared cooldown
→ update health
→ circuit breaker decision
→ suppress duplicate retries
→ select fallback provider/model
→ resume current run
```

Never:

```text
429 → hammer same provider repeatedly
```

Test:

```text
20–100 concurrent runs
→ burst 429
→ one shared cooldown
→ no retry storm
→ fallback works
→ runs remain alive
```

---

# 8. Phase 6 — Adaptive Retry

Different errors need different recovery:

```text
429      → Retry-After aware
5xx      → exponential backoff + jitter
timeout  → adaptive timeout/network retry
empty    → output recovery
malformed→ repair + corrective prompt
tool     → argument repair / alternate tool
verify   → continuation, NOT provider retry
```

Global recovery budget:

```text
max_retry_time
max_retry_count
max_recovery_cost
```

---

# 9. Phase 7 — Weak Model Resilience

Handle:

```text
empty response
"..."
invalid JSON
wrong tool
missing argument
wrong type
repeated tool
same answer repeated
premature done
```

Pipeline:

```text
detect
→ classify
→ repair
→ revalidate
→ retry
→ alternate strategy
→ fallback model
→ continue
```

Important: weak-model recovery must be **semantic**, not only JSON syntax repair.

---

# 10. Phase 8 — Completion Verifier as Terminal Gate

Current verifier must become authoritative.

Flow:

```text
model says DONE
→ completion verifier
→ acceptance criteria
→ artifacts
→ test/build status
→ tool consistency
→ quality gates
```

If incomplete:

```text
VERIFICATION_FAILED
→ RECOVERING
→ CONTINUING
```

Only verifier-passed runs may become `COMPLETED`.

This phase directly fixes the problem:

```text
model yếu → nói xong → task thực tế chưa xong
```

---

# 11. Phase 9 — Stalled Run Watchdog + Reconciliation

Detect:

```text
no event for N seconds
same tool repeated
same model output repeated
same workflow step repeated
budget increasing but no artifact
```

Classify:

```text
slow
stalled
looping
recovering-stuck
```

Recovery:

```text
checkpoint
→ nudge
→ strategy switch
→ model fallback
→ safe fail
```

Reconciliation worker scans:

```text
RUNNING with no heartbeat
WAITING_PROVIDER expired
WAITING_TOOL expired
RECOVERING stuck
checkpoint with no worker
```

---

# 12. Phase 10 — Stream / UI Recovery

Transport is presentation only.

```text
WebSocket disconnect
≠ run failed
```

Reconnect:

```text
GET current run state
+ replay missing events
```

User-facing states:

```text
⏳ Model đang cooldown
🔄 Đang chuyển model dự phòng
🧠 Agent vẫn đang xử lý
🔧 Đang sửa tool call
🧪 Đang verify
⏸ Đang chờ approval
✅ Hoàn thành
```

Không hiển thị `Something went wrong` nếu durable run chưa ở `FAILED`.

---

# 13. Phase 11 — Quality Gate Engine

Skill khai báo:

```yaml
quality-gates:
  - tests_pass
  - build_pass
  - acceptance_criteria
```

Gate types:

```text
test
build
lint
security
file
artifact
schema
review
approval
```

Flow:

```text
execute → verify → gates → complete
```

---

# 14. Phase 12 — Native AgentKit Skill Runtime

Chuyển từ:

```text
skill metadata → prompt injection
```

sang:

```text
SkillSpec
SkillResolver
SkillExecutor
SkillContext
SkillResult
```

`SkillContext`:

```text
run
agent
workflow
tools
memory
budget
artifacts
policy
```

---

# 15. Phase 13 — Native Workflow DAG

Workflow example:

```yaml
name: fix
steps:
  - analyze
  - reproduce:
      depends_on: [analyze]
  - patch:
      depends_on: [reproduce]
  - test:
      depends_on: [patch]
  - review:
      depends_on: [test]
  - complete:
      depends_on: [review]
```

Support:

```text
sequence
parallel
condition
retry
timeout
approval
rollback
subworkflow
```

---

# 16. Phase 14 — Convert Existing `/gc:` into Real Workflows

`/gc:plan`:

```text
understand → inspect → analyze → plan → verify → artifact
```

`/gc:fix`:

```text
reproduce → diagnose → root-cause → patch → test → review → complete
```

`/gc:cook`:

```text
load plan → implement → test → repair → review → gates → artifact
```

`/gc:review`:

```text
inspect → correctness → security → performance → maintainability → verdict
```

**Quan trọng:** `/gc:cook` và `/gc:fix` phải chạy bằng durable workflow run, không chỉ dựa trên prompt instructions.

---

# 17. Phase 15 — Hook Engine

Hooks:

```text
before_run
after_run
before_llm
after_llm
before_tool
after_tool
before_checkpoint
after_checkpoint
before_complete
after_complete
on_error
on_rate_limit
```

Mỗi hook có:

```text
timeout
priority
permissions
failure policy
```

---

# 18. Phase 16 — Artifact Engine

Artifact types:

```text
plan
patch
code
research
review
test-report
architecture
ADR
migration
deployment-plan
```

Metadata:

```text
artifact_id
run_id
type
version
parent
checksum
created_at
status
```

Traceability:

```text
requirement
→ plan
→ implementation
→ tests
→ review
```

---

# 19. Phase 17 — Native Kit Runtime

Kit structure:

```text
kit.yaml
skills/
agents/
workflows/
hooks/
templates/
tests/
```

Commands:

```text
/gc:kit list
/gc:kit inspect <name>
/gc:kit install <name>
/gc:kit update <name>
/gc:kit rollback <name>
```

Support:

```text
version pinning
checksum
rollback
compatibility
```

Project lockfile:

```text
.goclaw/kit.lock
```

---

# 20. Phase 18 — Skill Dependency + Tool Permission Enforcement

Resolve:

```text
missing dependency
version conflict
cycle
required tool
required capability
```

`allowed-tools` phải được **enforce tại runtime**, không chỉ viết vào prompt.

Flow:

```text
Skill requests tool
→ Policy Engine
→ allow / deny / approval
```

---

# 21. Phase 19 — More `/gc:` Commands

Engineering:

```text
/gc:architect
/gc:research
/gc:debug
/gc:test
/gc:security
/gc:refactor
/gc:docs
/gc:optimize
/gc:mission
/gc:deploy
```

Control:

```text
/gc:status
/gc:runs
/gc:doctor
/gc:resume
/gc:retry
/gc:pause
/gc:cancel
/gc:fork
/gc:approve
/gc:explain
```

---

# 22. Phase 20 — Model Capability Profiles

Profile:

```yaml
model: example
capabilities:
  coding: 0.92
  reasoning: 0.91
  tool_calling: 0.88
  structured_output: 0.95
  long_context: 0.90
reliability:
  success: 0.94
  timeout: 0.03
```

Score đến từ:

```text
static metadata
+ runtime telemetry
+ eval benchmark
```

---

# 23. Phase 21 — Model Router 2.0

Router xét:

```text
task complexity
required capabilities
provider health
historical success
cost
latency
context size
```

Output:

```text
primary
fallback
emergency
```

Model routing không chỉ tối ưu quality mà tối ưu:

```text
success probability × cost × latency
```

---

# 24. Phase 22 — Strategy Router

Strategies:

```text
direct
plan-first
decompose
research-first
test-driven
subagent
parallel
```

Ví dụ:

```text
simple task → direct
complex task → plan-first
weak model + complex → decompose
high risk → planner + coder + reviewer
```

---

# 25. Phase 23 — Smart Retry

`/gc:retry` không được replay nguyên xi.

Mỗi retry có thể thay đổi:

```text
model
strategy
context
tool order
workflow branch
```

Ví dụ:

```text
attempt 1 → direct
attempt 2 → plan-first
attempt 3 → stronger model
attempt 4 → subagent
```

---

# 26. Phase 24 — Experience Store

Lưu mỗi run:

```text
task type
skill
workflow
model
provider
strategy
success
cost
latency
recovery events
artifact
```

Task tương tự:

```text
retrieve experience
→ recommend successful strategy
```

Không cho experience tự sửa security/policy.

---

# 27. Phase 25 — Structured Agent Handoff

Contract:

```yaml
task:
context:
constraints:
artifacts:
acceptance_criteria:
budget:
deadline:
```

Agent nhận phải acknowledge contract.

---

# 28. Phase 26 — Dynamic Teams / Competitive Agents / Jury

Dynamic formation:

```text
simple → 1 agent
medium → planner + coder + tester
large → architect + researchers + coder + reviewer + security
```

Competitive mode:

```text
strategy A
strategy B
strategy C
→ judge
→ best result
```

Jury cho task high-risk:

```text
implementation
→ independent reviewers
→ consensus
```

---

# 29. Phase 27 — Mission 2.0 + Hibernation

Mission object:

```text
goal
milestones
tasks
agents
artifacts
budget
deadline
policy
status
```

Hibernation:

```text
RUNNING
→ checkpoint
→ HIBERNATED
→ wake condition
→ RESUME
```

Wake khi:

```text
provider cooldown ends
human approval arrives
external task completes
schedule triggers
resource pressure clears
```

---

# 30. Phase 28 — Agent Time Travel / Fork

Commands:

```text
/gc:inspect <run-id>
/gc:replay <run-id>
/gc:fork <run-id> --from <checkpoint>
```

Use:

```text
debug model
compare models
compare strategies
reproduce bugs
```

---

# 31. Phase 29 — Predictive Failure Detection

Risk signals:

```text
retry count
tool repetition
latency growth
empty outputs
context growth
token burn
provider health
```

High risk:

```text
checkpoint
→ fallback model
→ strategy switch
→ verifier/subagent
```

---

# 32. Phase 30 — Context + Project Intelligence

Dynamic context layers:

```text
L0 task
L1 relevant files
L2 architecture
L3 memory
L4 knowledge
L5 history
```

Cần:

```text
relevance ranking
deduplication
compression
pruning
budgeting
```

Project index:

```text
symbols
files
dependencies
API
config
tests
docs
```

Retrieval:

```text
keyword + semantic + graph + recency
```

---

# 33. Phase 31 — Git / Worktree / Impact Analysis

Coding run:

```text
main
→ worktree/<run-id>
```

Agent sửa trong isolated worktree.

Trước khi modify:

```text
changed symbols
→ dependency graph
→ affected packages
→ affected tests
→ affected APIs
```

Sau khi verify:

```text
merge
or
abort/discard
```

---

# 34. Phase 32 — Regression Intelligence

Sau fix:

```text
root cause
→ regression test
→ incident signature
→ experience
```

Bug tương tự:

```text
retrieve incident
→ recommend prior successful strategy
```

---

# 35. Phase 33 — Security / Policy Runtime

Policy controls:

```text
filesystem
shell
network
git
deploy
secrets
subagents
```

Risk:

```text
LOW
MEDIUM
HIGH
CRITICAL
```

Autonomy:

```text
LOW      → automatic
MEDIUM   → automatic + audit
HIGH     → approval
CRITICAL → explicit approval
```

---

# 36. Phase 34 — Skill Sandbox + Package Security

Skill manifest:

```yaml
permissions:
  filesystem: project
  network: none
  shell: restricted
  secrets: none
```

Package:

```text
manifest
checksum
signature
dependencies
permissions
```

Install:

```text
download → verify → inspect → install
```

Không cho skill tự mở rộng permission.

---

# 37. Phase 35 — Evaluation Harness

```text
evals/
  coding/
  debugging/
  planning/
  tool-use/
  reasoning/
  reliability/
  security/
  regression/
```

Metrics:

```text
task_success
tool_success
completion_accuracy
recovery_rate
cost
latency
```

Mỗi release phải chạy regression baseline.

---

# 38. Phase 36 — Simulation + Chaos Testing

Command:

```text
/gc:simulate <task>
```

Inject:

```text
429
500
timeout
empty stream
disconnect
malformed JSON
tool failure
slow tool
context overflow
worker crash
```

Pass criteria:

```text
no data loss
no duplicate side effect
run resumes
state remains correct
```

---

# 39. Phase 37 — Idempotent Side Effects

Side-effect tools phải có:

```text
idempotency_key
```

Áp dụng cho:

```text
send message
payment
API mutations
database migration
deployment
```

Retry không được tạo duplicate side effect.

---

# 40. Phase 38 — Resource-Aware Scheduler

Scheduler xét:

```text
CPU
RAM
queue depth
provider capacity
tenant limits
budget
```

Có:

```text
fair scheduling
priority
backpressure
```

Mục tiêu: task nặng của một user không làm nghẽn toàn hệ thống.

---

# 41. Phase 39 — Cost Governance

Budget:

```text
tokens
time
tool calls
model spend
subagents
```

Khi gần vượt budget:

```text
cheaper model
smaller context
decompose
pause
ask approval
```

---

# 42. Phase 40 — Self-Improvement Guardrails

Pipeline:

```text
observe
→ propose
→ sandbox
→ evaluate
→ benchmark
→ approve
→ adopt
```

Không cho agent tự sửa production policy/security trực tiếp.

---

# 43. Phase 41 — Documentation

Hoàn thiện:

```text
docs/agent-runtime.md
docs/recovery.md
docs/gc-commands.md
docs/agentkit.md
docs/workflows.md
docs/skills.md
docs/kits.md
docs/checkpoints.md
docs/model-routing.md
docs/security.md
docs/evals.md
```

---

# 44. Phase 42 — Backward Compatibility / Migration

Không phá:

```text
existing sessions
providers
tools
memory
APIs
configuration
```

Migration phải:

```text
versioned
idempotent
rollback-safe
```

---

# 45. Phase 43 — Final Integration Tests

## Test A — 429 storm

```text
100 concurrent runs
→ provider 429 burst
```

Expect:

```text
shared cooldown
no retry storm
fallback
run survives
```

## Test B — weak model

```text
malformed JSON
wrong tool
bad args
empty output
loop
premature done
```

Expect:

```text
repair
retry
strategy switch
fallback
continue or explicit failure
```

## Test C — stream disconnect

```text
run active
→ WebSocket disconnect
→ reconnect
```

Expect:

```text
run continues
state correct
event gap replayed
```

## Test D — worker crash

```text
kill worker mid-task
```

Expect:

```text
checkpoint
requeue
resume
```

## Test E — side effect timeout

```text
server processed request
client timed out
retry
```

Expect:

```text
no duplicate effect
```

## Test F — false completion

```text
model says DONE
acceptance incomplete
```

Expect:

```text
NOT COMPLETED
→ continue
```

---

# 46. Implementation Order

Làm đúng thứ tự này:

```text
1. Durable Run State Machine
2. Event Store
3. Checkpoint 2.0
4. Recovery Engine
5. 429/provider fallback integration
6. Empty/malformed output recovery
7. Completion verifier terminal gate
8. Stream recovery
9. Watchdog + reconciliation
10. Quality gates
11. Native Workflow Engine
12. Native Skill Runtime
13. Hook Engine
14. Artifact Engine
15. Kit versioning
16. Tool permission enforcement
17. More /gc commands
18. Model capability profiles
19. Model Router 2.0
20. Strategy Router
21. Experience Store
22. Structured handoff
23. Dynamic teams
24. Competitive agents + jury
25. Mission 2.0
26. Hibernation 2.0
27. Time travel/fork
28. Predictive failure detection
29. Context/project intelligence
30. Evals
31. Simulation/chaos
32. Security hardening
33. Production hardening
34. Documentation + migration
```

---

# 47. Milestones

## M1 — Reliability Complete

```text
[ ] 429 recoverable
[ ] timeout recoverable
[ ] empty output recoverable
[ ] malformed tool call recoverable
[ ] premature completion recoverable
[ ] stream disconnect safe
[ ] worker crash resumable
```

## M2 — Durable Agent Runtime

```text
[ ] state machine
[ ] event replay
[ ] checkpoints
[ ] reconciliation
[ ] durable /gc:cook
[ ] durable /gc:fix
```

## M3 — AgentKit Native Runtime

```text
[ ] Kit
[ ] Skill
[ ] Workflow
[ ] Hook
[ ] Artifact
[ ] Quality Gate
[ ] Permission enforcement
```

## M4 — Intelligent Routing

```text
[ ] capability profiles
[ ] model router
[ ] fallback
[ ] strategy switch
[ ] experience store
```

## M5 — Multi-Agent

```text
[ ] handoff
[ ] teams
[ ] competition
[ ] jury
[ ] budget governance
```

## M6 — 2026 Agent Platform

```text
[ ] mission
[ ] hibernation
[ ] time travel
[ ] predictive recovery
[ ] evals
[ ] chaos
[ ] self-improvement
```

---

# 48. Final `/gc:` UX

Engineering:

```text
/gc:plan <task>
/gc:cook <plan>
/gc:fix <issue>
/gc:review <target>
/gc:research <topic>
/gc:architect <system>
/gc:debug <issue>
/gc:test <target>
/gc:security <target>
/gc:refactor <target>
/gc:mission <goal>
```

Control:

```text
/gc:status
/gc:runs
/gc:doctor
/gc:resume
/gc:retry
/gc:pause
/gc:cancel
/gc:fork
/gc:approve
/gc:explain
```

UX cuối cùng phải đơn giản:

```text
/gc:cook thêm tính năng X
```

Backend tự xử lý:

```text
plan
→ execute
→ recover
→ verify
→ artifact
→ complete
```

---

# 49. Golden Rules

```text
LLM proposes
Agent executes
Runtime observes
Verifier decides
Policy controls
Checkpoint persists
Event Store records
Recovery continues
```

Không:

```text
LLM says done → COMPLETED
```

Không:

```text
transport failed → task failed
```

Không:

```text
429 → hammer provider
```

Không:

```text
retry → duplicate side effect
```

---

# 50. Final Target

GoClaw sau roadmap này phải trở thành:

```text
                  GOCLAW 2026
                       │
      ┌────────────────┼────────────────┐
      │                │                │
 AgentKit Runtime  Reliability     Model Intelligence
      │                │                │
 Kit / Skill       Recovery        Routing / Fallback
 Workflow          Checkpoint      Capability
 Hook              Event Store     Experience
 Artifact          Watchdog        Strategy
      │                │                │
      └────────────────┼────────────────┘
                       │
                Multi-Agent Runtime
                       │
             Teams / Jury / Mission
                       │
                    User/API
```

**Kết quả cuối:** model mạnh thì GoClaw chạy nhanh và ít recovery; model yếu thì GoClaw dùng planning, decomposition, repair, retry, fallback, strategy switching và verification để giữ task sống lâu nhất có thể thay vì bỏ cuộc sớm.
