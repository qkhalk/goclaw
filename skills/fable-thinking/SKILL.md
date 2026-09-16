---
name: fable-thinking
description: >-
  Creative reframing through metaphor and fable: turn a technical problem into
  a short story to unlock angles that literal analysis misses, then map every
  insight back to concrete actions. Use when a team is stuck in the same
  framing, or when explanation and buy-in matter as much as the solution.
  Keywords: metaphor, analogy, narrative, lateral thinking, storytelling.
  Dùng khi cần góc nhìn mới, suy nghĩ sáng tạo, hoặc giải thích vấn đề bằng
  ẩn dụ và câu chuyện.
license: MIT
version: 1
---

# Fable Thinking

Reframe the problem as a story, let the story reveal structure, then translate
the story's lessons back into engineering actions. The fable is scaffolding —
the output is concrete.

## When to use
- The team keeps circling the same two options and needs a fresh frame.
- A system's dynamics (feedback loops, incentives, bottlenecks) matter more
  than its components.
- The solution is right but stakeholders do not feel why.

## When NOT to use
- Metrics, logs, or a failing test already pinpoint the answer.
- The user wants speed and precision, not perspective — use `problem-solving`
  or `debug` instead.
- The metaphor would obscure compliance, security, or numeric detail.

## Workflow
1. Strip the problem to its essence in one sentence without jargon: who wants
   what, what blocks them, what has been tried.
2. Choose a metaphor domain with genuinely similar dynamics: an ecosystem, a
   market, a city, an organism, traffic, a kitchen. Match dynamics, not
   looks (queues match queues; feedback loops match feedback loops).
3. Write a 5-10 sentence fable: characters map to components or roles, the
   conflict to the constraint or bug, and the resolution must arise from the
   domain's own logic — no deus ex machina.
4. Harvest insights: list what the characters had to learn, give up, or
   rearrange. Ask "what is the story warning us about?"
5. Map every insight back: "the ferryman charging per crossing = per-request
   billing exposing our chatty API". Discard insights with no concrete
   counterpart.
6. Turn surviving insights into 2-4 actionable items, each with an owner
   role and a way to verify it worked.

## Output
The fable (short), an insight-to-action mapping table, and the resulting
concrete next actions. Never ship the fable without the mapping.

## Routing
- Actions need evaluation as a set of options → `brainstorm`.
- The fable is itself the deliverable (persuasion, docs, talk) → `copywriting`.
- System-shape conclusions need a real design → `architect` or `design`.

## Guardrails
- One metaphor per session; mixed metaphors produce noise, not insight.
- Flag the metaphor as a tool, never present story-logic as evidence.
- If no insight survives the mapping step, say so plainly rather than
  inventing depth.
