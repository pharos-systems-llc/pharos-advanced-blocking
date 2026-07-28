---
name: dev-server
description: Running the marketing site's Astro dev server in this sandboxed environment — known failure modes and fixes
metadata:
  type: runbook
---

# Marketing Site Dev Server

The marketing site (`marketing/`) is an Astro 7 + MDX site. Like all other tooling in this project, run it via Podman, not a host Node install — see `AGENTS.md` for the general containerization rule. This runbook exists because getting the dev server actually running here hit three distinct, non-obvious failures in practice; each is real and each cost real debugging time.

## The working invocation

```bash
podman run --rm --security-opt seccomp=unconfined \
  -p 4321:4321 \
  -v "$(pwd)/marketing:/workspace:z" \
  -w /workspace \
  public.ecr.aws/docker/library/node:24-bookworm \
  sh -c "npm install && node ./node_modules/.bin/astro dev --host 0.0.0.0 --force"
```

Run this with `run_in_background: true` (or your tool's equivalent) — it's a long-running server, not a one-shot command. Poll `curl -sf http://localhost:4321/pharos-advanced-blocking/` in a loop until it responds, rather than assuming it's ready immediately.

Use **Node 24**, not Node 22 — this project standardizes on 24 for the marketing site.

## Failure 1: Bash sandbox silently kills any port-binding process

If your Bash tool runs commands in a sandbox by default, a process that tries to `listen()` on a port (like `astro dev` binding to 4321) can be killed almost immediately after starting, with no useful error — just a bare `Terminated` message right after the process's own startup banner. This looks like an application bug; it isn't.

**Fix**: run the dev-server-launching command with your tool's sandbox explicitly disabled (e.g. Claude Code's Bash tool has a `dangerouslyDisableSandbox: true` parameter for exactly this). Verify the theory first with a trivial test if you're unsure — a minimal `node -e "require('http').createServer(...).listen(PORT)"` inside the same container reproduces the same silent kill if the sandbox is the cause, and rules it out if it isn't.

## Failure 2: stale Astro lock file after a force-killed container

If a previous dev server instance was killed uncleanly (e.g. `podman stop`/SIGKILL on a container that didn't get to clean up), Astro leaves a stale PID/lock file behind (in `node_modules/astro`'s runtime state, on the bind-mounted volume, so it persists across container runs). The next `astro dev` invocation then fails fast with:

```
Another astro dev server is already running.
  URL:  http://localhost:4321
  PID:  <stale PID>
Run `astro dev stop` to stop it, or use `astro dev --force` to replace it.
```

**Fix**: always pass `--force` to `astro dev` in this environment (already included in the working invocation above) rather than trying to track down and clean the stale lock file manually.

## Failure 3: `npm run dev` (the package.json script wrapper) dies for unrelated reasons

Even with the sandbox and lock-file issues both fixed, running the dev server via `npm run dev -- --host 0.0.0.0 --force` (i.e. through the `package.json` script wrapper) can still die silently right after printing Astro's telemetry banner — a bare `Terminated` again, before the server ever gets far enough to print its listening URL. The root cause was never fully isolated (possibly npm's own signal-handling/child-process wrapping in this container environment), but it's specific to the `npm run` wrapper.

**Fix**: bypass the wrapper and invoke `astro` directly, as in the working invocation above (`node ./node_modules/.bin/astro dev ...` instead of `npm run dev -- ...`). This reliably works where the `npm run` wrapper doesn't.

## If you hit something new

Verify hypotheses with the smallest possible reproduction before changing real project files — e.g. a trivial `sleep 30` background command isolates "is backgrounding broken right now" from "is this specific command broken," and a minimal HTTP server isolates "is port-binding broken" from "is Astro/npm broken." Don't guess at a fix and iterate on the real command in a loop.
