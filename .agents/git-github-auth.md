---
name: git-github-auth
description: Git/GitHub authentication issues specific to sandboxed agent environments
metadata:
  type: runbook
---

# Git/GitHub Authentication in Sandboxed Environments

Two real, non-obvious authentication blockers have been hit in this environment. Both are environment/permissions issues, not repository problems — don't spend time investigating the repo itself when you see these.

## `git push`/`git fetch` fails with "Network is unreachable" on port 22

```
ssh: connect to host github.com port 22: Network is unreachable
fatal: Could not read from remote repository.
```

This means outbound SSH (port 22) is blocked in this sandbox, even though HTTPS (443) works fine — which is why `gh` CLI commands (API calls, `gh release`, `gh run`, etc.) work throughout a session while a raw `git push`/`git fetch` over the SSH remote fails. Check `git remote -v`: if it shows `git@github.com:...`, that's the SSH form.

**Fix:**
```bash
git remote set-url origin https://github.com/<owner>/<repo>.git
gh auth setup-git
```

`gh auth setup-git` configures git to use `gh`'s existing authenticated credentials as a credential helper for `github.com`, so the subsequent HTTPS push/fetch doesn't prompt for a username/password it has no way to collect non-interactively.

## Push rejected: "refusing to allow an OAuth App to create or update workflow ... without `workflow` scope"

```
! [remote rejected] <branch> -> <branch> (refusing to allow an OAuth App to create or update workflow `.github/workflows/<file>.yml` without `workflow` scope)
```

GitHub requires the `workflow` OAuth scope specifically to push any commit that touches a file under `.github/workflows/`. The `gh` CLI's default authenticated token often doesn't have this scope.

**Fix:** this requires the human user to complete an interactive step — it cannot be done by an agent alone:
```bash
gh auth refresh -h github.com -s workflow
```
This prints a one-time code and a `https://github.com/login/device` URL. The user must open that URL, enter the code, and authorize it in their browser. `gh auth refresh` blocks (and can time out, `context deadline exceeded`, if the browser step isn't completed quickly) — tell the user the exact code and URL, then wait for them to confirm before retrying the push. After they confirm, verify the scope actually landed with `gh auth status` (look for `workflow` in the `Token scopes` line) before retrying, rather than assuming success.
