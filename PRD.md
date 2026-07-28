# Product Requirements Document (PRD): Pharos Advanced Blocking (`pab`)

## 1. Executive Summary & Objective

**Pharos Advanced Blocking (`pab`)** is a statically linked command-line interface (CLI) tool written in Go, designed to manage, validate, and synchronize Technitium DNS Server **Advanced Blocking App** configurations (`dnsApp.config`). 

The tool is built **CLI-first** and structured as an **AI-agent-friendly** tool, allowing human operators to interact with it via a rich Terminal User Interface (TUI), while permitting AI coding assistants and automation scripts to use structured JSON payloads and non-interactive parameters.

---

## 2. Core Architecture & Philosophy

1.  **Decoupled & GitOps Ready**: The tool works entirely **on disk** when editing configurations. It has zero coupling with Git binaries, allowing users to choose their own Git deployment, staging, or pipeline engine.
2.  **View First, Edit Second**: The primary interactive user workflow is optimized for scanning, querying, and checking status before writing changes to disk or publishing to the servers.
3.  **Cross-Compilation Target**:
    *   `linux/amd64`
    *   `linux/arm64`
4.  **No Runtime Dependencies**: Statically compiled binary requiring no local runtime environment (like Node.js, Python, or external interpreters).

---

## 3. Functional Requirements

### 3.1 Onboarding & Configuration Bootstrap (`pab init`)
`pab` does not automatically prompt for setup when `dnsApp.config` is missing — any command that needs a config (including launching the TUI with no arguments) fails with a plain "configuration file not found" error instead. Bootstrapping is instead an explicit, dedicated command:

```bash
pab init [--node <name>] [-f|--force] [--json]
```

`pab init`:
1.  **Resolves credentials** the same way `pab deploy`/`pab verify` do (environment variables for a single node, `secrets.json` for two or more — see §4.2) — prompting interactively for them if none are found, unless `--json` is set, in which case it errors clearly instead of prompting.
2.  **Fetches** the live Advanced Blocking configuration from the resolved Technitium node. If the server has no configuration yet, `pab init` falls back to a minimal template (mirrored at `dnsApp.config.example` in the repository root) instead of leaving the user with nothing.
3.  **When two or more nodes are configured**, checks that they agree on their current configuration before proceeding, and never auto-merges disagreeing nodes — it requires the user to explicitly pick one node as the source of truth via `--node`.
4.  **Previews** the resolved configuration (group count, group names, network mapping count), clearly labeled as not yet saved. `--json` prints the full configuration and exits without writing or prompting.
5.  **Requires explicit confirmation** (`[y/N]`, or `-f`/`--force` to skip it) before writing `dnsApp.config` to disk — including a structural diff against any existing local file on a re-run, so `pab init` never silently overwrites an existing configuration.

### 3.2 View & Query Modes (Human vs. AI Agent)
*   **TUI Mode (Default for Interactive TTY)**:
    *   Renders a beautiful ASCII table using `lipgloss` showing client IPs mapped to their blocking groups.
    *   Provides search/filter capabilities to find which group a specific IP belongs to.
    *   Lists the active groups and their configured blocklists/regex filters.
*   **Machine-Readable Mode (AI Agent Friendly)**:
    *   Adding the `--json` flag prints clean, parsed JSON outputs to stdout.
    *   Silences all interactive terminal visual animations (like spinners or progress bars).
    *   Supports the `--quiet` and `--no-color` flags.

### 3.3 Edit & Modification Workflow
*   **Add/Update Mapping**: Interactive console commands or direct arguments to map/reassign a client IP or range (CIDR) to a specific blocking group:
    ```bash
    pab map --ip 192.168.86.16 --group "Default+Richard"
    ```
*   **Remove Mapping**: Deletes a client IP mapping from the configuration:
    ```bash
    pab unmap --ip 192.168.86.18
    ```

### 3.4 Schema Validation Engine
Before saving any modifications to disk or posting to an API, the CLI runs a validation pass:
1.  **IP & Subnet Check**: Ensures every key in the client network map is a valid IPv4, IPv6, or CIDR network block (automatically rejecting MAC addresses or hostname strings, which cause Technitium crashes).
2.  **Group Integrity**: Verifies that any client mapping targets a group that actually exists in the `groups` section.
3.  **JSON Schema Check**: Validates blocklist URL structures and regex patterns.

### 3.5 Sync & Deploy Engine
Once validated, the CLI can sync the disk configuration to one or more target Technitium installations:
*   **Command**: `pab deploy [flags]`
*   **Dry Run**: `pab deploy --dry-run` performs a full structural diff between the disk configuration and the server API, listing what will be updated on each node without applying changes.
*   **Deployment Workflow**: After validating with `--dry-run`, run `pab deploy -f` to deploy the configuration to all configured nodes simultaneously. The array-based `secrets.json` schema defines all deployment targets as a unit, ensuring active-active deployment across all nodes.
*   **Node Discovery**: Use `pab list-nodes` to view all available node identities before deploying.
*   **Non-interactive Mode**: The `-f` (or `--force`) flag bypasses confirmation prompts for automation and scripting.

### 3.6 Verification Engine (`pab verify`)
`pab deploy` confirms a configuration was accepted by the Technitium API — it does not confirm the server is actually blocking anything. `pab verify` closes that loop:

```bash
pab verify --domain <domain> [--node <name>] [--group <name>] [--port <port>] [--json]
```

*   Sends a live DNS query for `--domain` directly to the target node's resolver (default port 53, Technitium's default DNS listener; `--port` overrides for a non-default deployment) — it does not rely on the system resolver or cache.
*   Automatically attributes `--domain` to a group by searching the locally loaded config's `blockedDomains`/`blockedRegex` entries; domains only covered by a remote block-list URL can't be attributed automatically (pab does not fetch and parse remote list contents), so `--group` must be passed explicitly in that case.
*   Compares the live response against the group's configured blocking behavior (`blockAsNxDomain` / `blockingAddresses`) and reports one of four outcomes: `blocked`, `not-blocked`, `inconclusive` (e.g. a sinkhole-style group with no `blockingAddresses` configured, so pab has nothing to compare the response against), or `error`.
*   `--node` is required only when two or more nodes are configured; unlike `pab deploy`'s fan-out to every node, `pab verify` targets exactly one node per invocation.
*   Exits non-zero unless the result is exactly `blocked`, so it chains safely after `pab deploy` in CI/CD: `pab deploy -f && pab verify --domain ads.example.com`.

---

## 4. Technical Specifications & Stack

### 4.1 CLI Interface (Go)
*   **CLI Router & Parser**: `github.com/spf13/cobra` for handling subcommands, flags, and arguments.
*   **TUI Engine**: `github.com/charmbracelet/bubbletea` for rich terminal interactions.
*   **Styling**: `github.com/charmbracelet/lipgloss` for padding, borders, and color definitions.
*   **Interactive Prompts**: Simple wizard-style prompts (e.g. the `pab init` credential flow) use Go's standard `bufio.NewReader(stdin)` pattern already established by `pab deploy`'s confirmation prompt — no third-party prompt library is used.

### 4.2 Security & Credential Store
Node count is the deciding factor for how credentials are supplied:

1.  **Environment Variables — exactly one node**: `TECHNITIUM_URL` and `TECHNITIUM_TOKEN` configure a single Technitium node, registered internally as `"default"`. This is the right choice for a single-server home lab or a CI/CD job that only ever talks to one node. There is no multi-node environment variable pattern — two or more nodes require `secrets.json` (below).
2.  **`secrets.json` — required for two or more nodes**: reads credentials from a local configuration directory (e.g. `~/.config/pab/secrets.json`).
    *   **Array-Based Schema**: The `secrets.json` file uses an array structure to define all deployment nodes as a unit, with an optional `name` field per node (falls back to a positional `node-<index>` name when omitted):
        ```json
        {
          "nodes": [
            {"name": "dns1", "url": "https://dns1.example.com:5385", "token": "api-token-dns1"},
            {"name": "dns2", "url": "https://dns2.example.com:5385", "token": "api-token-dns2"}
          ]
        }
        ```
    *   **Requirements**: The CLI will refuse to run and print a warning if this configuration file does not have strict permissions (`chmod 600`), preventing other system users from reading the file contents.
    *   **CI/CD use**: `secrets.json` is also the correct mechanism for multi-node CI/CD deployments — write the file from a CI secret to a temp path with `chmod 600` immediately before running `pab deploy`, rather than trying to express multiple nodes as environment variables.
3.  **Onboarding Wizard**: `pab init` prompts to securely enter node URL(s) and token(s). For a single node it prints the `export TECHNITIUM_URL=...`/`TECHNITIUM_TOKEN=...` lines for the user to add to their own shell profile (pab cannot persist environment variables on the user's behalf); for two or more nodes it writes directly to `~/.config/pab/secrets.json` in the array format above, with permissions set to `600` at creation time (not chmod'd after the fact).
4.  **Visible resolution**: because environment variables and `secrets.json` can both contribute nodes at once (e.g. one node from `TECHNITIUM_URL` plus more from `secrets.json`), `pab list-nodes` and `pab init` report the merge explicitly (e.g. "1 from environment variables + 2 from secrets.json = 3 total") rather than blending sources silently.

### 4.3 Plugin Extensibility (Phase 6 - Completed)
To support the future **Live Status Plugin** (e.g. displaying real-time queries and lease information across multiple nodes), the CLI core implements an RPC-like sub-process plugin engine:
*   A directory-based plugin loader (e.g., scanning `~/.config/pab/plugins` for executables starting with `pab-plugin-`).
*   Plugins are built as standalone executables (e.g. `pab-plugin-sample`), preserving our CGO-free, statically compiled mandate.
*   Dynamic command registration: The loader queries the plugin executable (e.g. `./plugin info`) to get JSON metadata describing the plugin and its commands, and dynamically registers these subcommands into Cobra's runtime registry. Execution routes the command directly to the plugin executable.

---

## 5. Build, Release, & Installation (Completed & Verified)

### 5.1 Compilation & Assembly (Completed)
The project uses Go's native cross-compilation capability. We have integrated **GoReleaser** and version linker injections to compile versions and commits.

### 5.2 Release Artifacts (Completed)
1.  **Static Binary**: Standalone compressed `.tar.gz` archive containing the binary.
2.  **Debian Package (`.deb`)**:
    *   Constructed via `goreleaser` and `nfpm`.
    *   Integrates with Debian-based systems' standard package registry.
    *   Exposes clean install and remove routes (`apt install ./pab.deb` / `apt remove pab`).

The release workflow (`.github/workflows/release.yml`) fails the job outright if a published release ends up with zero assets, so an empty release can no longer ship silently.

### 5.3 Installation Scripts (Completed)
*   **Bash Installer**: An `install.sh` script (mirrored in the Git repository and served from the marketing site) that:
    1.  Detects system CPU architecture (rejecting 32-bit x86 architectures).
    2.  Downloads the latest release binary matching the architecture from GitHub Releases.
    3.  Verifies the SHA256 checksum automatically against the official release manifest before extracting.
    4.  Prints manual Cosign signature verification instructions when `cosign` is present — the installer does not run `cosign verify-blob` automatically; verification is a manual follow-up step for now.
    5.  Extracts and installs the binary to `/usr/local/bin/pab` (using sudo) or falls back to local user installation in `~/.local/bin/pab`.
