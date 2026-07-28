# Google Antigravity Rules: Pharos Advanced Blocking (`pab`)

This file lives in `.agents/rules/` because that's where Antigravity's workspace-rules discovery looks (confirmed against Antigravity's own docs: `.agents/rules/` is the current default location, with `.agent/rules/` kept only for backward compatibility). Its content is a real, mechanical import — not a prose pointer a reader has to choose to follow — of the canonical, cross-tool rules at the repository root:

@/AGENTS.md

The import above (`@/AGENTS.md`) resolves relative to the repository root per Antigravity's documented `@filename` syntax, and its content is loaded here, not just linked. Podman/containerization requirements, git/repository decoupling, and Technitium API/security rules all live in that one file — kept in one place instead of duplicated per tool.
