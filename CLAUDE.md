@AGENTS.md

## Claude Code specifics

- For code changes, follow `.agents/workflows/pab-feature-loop.md`'s inner loop (test/compile/snapshot) before invoking the mob review panel (`.agents/review-panel.md`).
- For any multi-step task involving subagent delegation, see `.agents/workflows/agent-decomposition.md` first — it covers when to delegate to a cheaper model, how to specify tasks precisely enough for that to work, and the trust-but-verify rule for checking subagent output independently rather than accepting self-reports at face value.
- Running the marketing site locally: read `.agents/dev-server.md` first — it documents three real, previously-hit failure modes (stale Astro lock file, sandboxed port binding, the `npm run dev` wrapper dying) and their fixes.
- Git/GitHub auth issues in this environment (SSH unreachable, `workflow`-scope OAuth errors on workflow-file pushes): see `.agents/git-github-auth.md`.
