# Agent Rules: Pharos Advanced Blocking (`pab`)

Canonical, tool-agnostic instructions for any AI coding agent working in this repository (Claude Code, Google Antigravity, or others). This file follows the open [AGENTS.md](https://agents.md) convention: one set of rules, read natively where supported and imported where not, instead of hand-maintained duplicates per tool.

---

## 1. Containerized Development (Podman) — MANDATORY

- **No host tool installation**: never install Go (`golang-go`, `snap`, etc.) directly on the host. All compilation, dependency updates (`go get`, `go mod tidy`), and test execution MUST run inside Podman using the AWS ECR public mirror image, not Docker Hub (avoids rate limits):
  ```bash
  podman run --rm --security-opt seccomp=unconfined \
    -v "$(pwd):/workspace:z" \
    -w /workspace \
    public.ecr.aws/docker/library/golang:1.22-bookworm \
    <command>
  ```
- **`--security-opt seccomp=unconfined` is non-negotiable** on every `podman run` invocation — bypasses OCI sandbox restrictions that otherwise cause permission errors on this workstation (specifically, `bdflush` OCI permission errors).
- **`:z` mount suffix is required** for SELinux compatibility on the workspace volume mount.
- **Feedback loop targets**: unit tests 3-5s, binary compile 5-10s, GoReleaser snapshot 15-20s. If a command exceeds these, suspect container startup overhead or a cold image pull, not a real regression.

## 2. Code & Repository Decoupling

- **No git commands in application code**: `pab` itself must never shell out to `git` or attempt commits — it writes configuration directly to disk (`dnsApp.config` or the path given via `--config`), leaving Git entirely to the developer's external workflow.
- This is a constraint on the Go source `pab` produces, not on you as an agent — you may run `git`/`gh` directly in your own tool-use to manage branches, commits, and PRs for this repository. It only means: don't write Go code where the `pab` binary itself invokes git.

## 3. Technitium API & Security Rules

- **DHCP lease API schema**: `address` (IP), `hardwareAddress` (MAC, colon-formatted), `hostName` (Hostname), `comments` (Description). Lease deletion uses `/api/dhcp/scopes/removeReservedLease`, targeting `hardwareAddress`, not the IP.
- **Strict credentials guard**: the CLI must refuse to run and exit with a high-priority security error if `~/.config/pab/secrets.json` has Unix permissions weaker than `chmod 600`. This is enforced in `internal/config/security.go` and must not be weakened.

## 4. Where to find deeper, task-specific guidance

This file holds only what's true every session, everywhere. For anything longer or more situational, follow these pointers rather than expanding this file:

- **`.agents/dev-server.md`** — running the marketing site's Astro dev server (known pitfalls: stale lock files, sandboxed port binding, the `npm run dev` wrapper issue).
- **`.agents/workflows/agent-decomposition.md`** — how to decompose a task across an orchestrating agent, cheap-model subagents, and review-panel agents; when a task is mechanical enough to delegate cheaply vs. needs judgment; the trust-but-verify rule for subagent output.
- **`.agents/workflows/pab-feature-loop.md`** — the fast inner loop (test/compile/snapshot) and mob review gate for code changes.
- **`.agents/review-panel.md`** — the 7-persona mob programming review panel definition, invoked for code review before merge.
- **`.agents/builder.md`** — the Go build/test expert persona for the fast inner loop.
- **`.agents/git-github-auth.md`** — troubleshooting git/gh authentication issues specific to sandboxed environments (SSH unreachable, workflow-scope OAuth requirements).
- **`DEVELOPMENT.md`** — implementation roadmap, TUI feature specs, release process, and marketing-site accuracy requirements.
- **`PRD.md`** — product requirements and current feature status.

## 5. Principle: Documentation > Marketing

If project documentation and marketing claims contradict each other, the documentation (and the actual code) is authoritative — fix the marketing copy, not the other way around. This has mattered in practice: this repository has shipped marketing claims for features that didn't yet exist in code, and separately shipped internal doc links that silently 404'd because they weren't cross-checked against actual routing configuration.
