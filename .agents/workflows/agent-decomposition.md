---
name: agent-decomposition
description: How to decompose a task across an orchestrating agent, cheap-model subagents, and review-panel agents
metadata:
  type: workflow
---

# Agent Decomposition Pattern

`pab-feature-loop.md` covers the build/test/review cycle for a single change. This document covers a layer above that: how to structure a larger piece of work (a feature spanning multiple files, a multi-round investigation, a documentation overhaul) across multiple agents with different cost/capability tradeoffs, without losing quality or correctness along the way.

## The three roles

**Orchestrator** (the agent you're talking to directly) — does planning, architectural investigation, and anything requiring judgment or synthesis across multiple sources. Verifies uncertain premises directly (reads real docs, runs real commands) rather than assuming. Writes precise, unambiguous specs for mechanical work, then delegates execution.

**Execution subagents** (can be a cheaper/faster model) — given a fully-specified, mechanical task: exact files to change, exact before/after content, exact verification commands to run. Good for narrow, single-file or few-file changes where the *design* decision has already been made by the orchestrator and only *typing it out correctly* remains.

**Review-panel agents** (`review-panel.md` and similar persona-based panels) — independent quality/security/UX gates, run separately from whoever did the implementation. Their value is specifically in being independent: an agent reviewing its own prior work (or an orchestrator re-reading its own reasoning) is prone to confirming what it already believed.

## When a task is "mechanical enough" to delegate cheaply

Delegate to a cheap-model subagent when you can write a spec that includes the *exact* final content (not just a description of the goal) and the task doesn't require discovering anything new about the codebase or the tools involved. Good signal: if you find yourself writing "and if X doesn't work, try Y instead" into the spec, the task still has an undecided design question in it — resolve that yourself first, or the subagent will end up making an architectural call it's not positioned to make well.

**Do the investigation yourself first when:**
- The premise of the task is unverified (e.g., "does this framework/API actually support the approach we're about to specify?"). A spec built on a wrong assumption fails regardless of how precisely it's written — this happened once in this project's history when a TOC-repositioning task assumed Astro's `layout:` frontmatter convention supported named slots; it doesn't, and no amount of spec precision would have made that work. The fix was for the orchestrator to verify against the framework's actual documentation, not to write an even more detailed spec.
- The task requires synthesizing information from multiple existing files into one coherent new artifact (e.g. consolidating scattered rules into a single canonical doc) — this needs holistic judgment about what to keep, merge, or cut.

**Delegate to a cheap model when:**
- The change is confined to one or a small, explicit set of files, with literal before/after content specified.
- Verification is mechanical (run a command, check output matches an expected pattern) rather than requiring judgment.

## Trust but verify — the load-bearing rule

Never accept a subagent's self-report of success as sufficient. After any dispatch, independently re-check the actual result yourself:
- Read the file(s) it changed directly, not just the diff it quotes back to you.
- Re-run the verification command yourself rather than trusting the pasted output.
- For anything with a live/running component (a dev server, a deployed page, a CI run), hit it directly (`curl`, `gh run view`, etc.) rather than trusting a description of what it should show.

This has caught real problems in practice: a subagent's plausible-looking self-report of success once masked an actual bug (a wrapper component that silently dropped content instead of routing it correctly) that only surfaced on direct inspection of the live rendered output.

## The resumable-panel-agent pattern

For any work that spans multiple rounds of design questions over time (not just one dispatch-and-done task), prefer resuming the *same* long-lived agent across those rounds rather than re-spawning a fresh one each time. A resumed agent accumulates its own findings and can reference its own prior conclusions without re-litigating them from scratch — this matters most for iterative content/design review (a panel of personas revisiting the same evolving document across many questions) but generalizes to any multi-round investigation.

Pair this with a single, continuously-appended scratchpad document for that investigation (rather than fragmenting findings across many small files) as long as it's genuinely one continuous inquiry. Start a new document only when the topic is genuinely separate, not just a new question about the same thing.
