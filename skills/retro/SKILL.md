---
name: retro
description: >-
  Data-driven engineering retrospective from git history: commit cadence,
  churn hotspots, test-to-code ratio, and fix-vs-feature mix, producing
  findings plus 2-3 concrete process improvements. Use at milestone ends or
  when delivery feels slow. Keywords: retrospective, churn, git stats, process
  improvement, hoi so, cai tien quy trinh. Dùng khi cần nhìn lại quy trình làm
  việc dựa trên dữ liệu git thay vì cảm tính.
license: MIT
version: 1
---

# Retro

Mine read-only git history and repo stats for signals — commit cadence, churn hotspots, test-to-code ratio, rework patterns — then propose two or three concrete, checkable process improvements.

## When to use
- End of a milestone, sprint, or project phase
- Delivery feels slow or rework keeps happening and you want evidence
- Before changing team conventions, to ground the debate in data
- Comparing two periods (before/after a process change)

## When NOT to use
- You need a status snapshot — that is `watzup`
- The ask is code quality of one PR — that is `review`
- The repo has almost no history (under ~2 weeks of commits)

## Workflow
1. Define the period and scope: branch, date range, directories in and out.
2. Gather cadence with read-only `exec`: commits per day/week, distribution over authors/agents, type mix from conventional prefixes (feat/fix/chore/docs).
3. Measure churn: files changed most often in the period; correlate churn hotspots with fix commits touching the same files.
4. Estimate test-to-code ratio: compare added/changed lines under test paths vs non-test paths using diff stats.
5. Measure rework: commits that revert or heavily amend recent commits; fix commits referencing recently shipped features.
6. Pull qualitative context: journals, notes, `decision-log` entries, review threads from the period.
7. Synthesize 3-5 findings, each with evidence (numbers, file lists) and a plausible mechanism.
8. Propose 2-3 improvements; each must be concrete, cheap to try, and checkable next period — name the metric that should move.
9. Save the report in the workspace retro area and schedule the follow-up comparison (optionally via `cron`).

## Output
A retro report: period and scope; findings (each with signal, evidence, mechanism); improvements (each with change, owner, expected metric movement); an appendix listing the exact read-only commands run.

## Routing
- Status/what-happened reporting -> `watzup`
- An improvement requires architectural change -> `architect`
- Improvements land as tasks -> `plan`, then `cook`
- Persist conclusions for future sessions -> `decision-log`

## Guardrails
- Read-only analysis: never rewrite history to make stats look better
- Correlation is not cause: label mechanisms as hypotheses
- Analyze the process, never rank or shame individuals
- Flag small samples: a signal resting on fewer than 5 events is noise until proven
- Every improvement names the metric that will prove it worked
