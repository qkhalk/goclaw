---
name: ask
description: >-
  Ask users effective questions: when to ask versus decide alone, one question at a time, concrete
  options with trade-offs and a recommendation, using the goclaw ask_options tool. Use when a
  decision is genuinely the user's, requirements conflict, or you risk building the wrong thing.
  Keywords: clarify, ask user, options, decision, requirements, question, hoi nguoi dung, lam ro
  yeu cau. Dùng khi cần hỏi người dùng để làm rõ yêu cầu hoặc quyết định quan trọng.
license: MIT
version: 1
---

# Ask

Get the information you actually need from the user with minimal friction and maximal signal.

## When to use
- A decision is genuinely the user's: product direction, scope cuts, irreversible operations, paid services.
- Requirements conflict or stay ambiguous after you exhausted the codebase, docs, and `docs-seeker`.
- You are about to invest significant work on an assumption you cannot verify yourself.

## When NOT to use
- The answer is discoverable from code, docs, or `research` — look first; asking is the last resort.
- The choice is conventional with an obvious default — pick the default, state it, and proceed.
- Asking would merely transfer your thinking work onto the user.

## Workflow
1. List what you are unsure about and what each answer would change. Questions that change nothing are noise.
2. Try to resolve each item yourself: search the repo, read configs, check sibling implementations. Only truly user-owned decisions remain.
3. Formulate ONE question. Multiple simultaneous questions produce shallow answers.
4. Provide 2-4 concrete options, each with a one-line trade-off. Put your recommended option first and label it recommended.
5. Ask via the `ask_options` tool when available (structured options, clean UX); otherwise ask in plain text with lettered options.
6. While waiting, do all independent work — never idle on the answer if other steps can proceed.
7. Record non-obvious outcomes in your notes or `decision-log`, including the options you rejected and why.

## Output
Either a resolved decision (chosen option plus rationale) or a single well-formed question with options — never an interrogation.

## Routing
- The question arose mid-implementation → return to `cook` or the active plan once answered.
- The decision is architectural and worth remembering → `decision-log`.
- Scope changed materially as a result → update `goclaw-kit` or `project-management` state.

## Guardrails
- Never ask what you can look up; never decide what only the user can decide.
- One question per round-trip; batch only if the options are truly independent.
- Destructive or irreversible operations always require explicit confirmation, even when you feel sure.
