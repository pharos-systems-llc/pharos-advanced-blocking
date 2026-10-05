package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/client"
	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/config"
	"github.com/spf13/cobra"
)

// maxCredentialAttempts bounds the interactive "fetch failed, re-enter
// credentials?" loop so a scripted/broken stdin can't spin forever.
const maxCredentialAttempts = 3

// newInitCmd defines the "pab init [--node <name>] [--config <path>] [-f|--force] [--json]"
// command: the credentials -> fetch -> [conditional multi-node consistency check] ->
// preview -> explicit-commit onboarding flow described in PRD.md §3.1/§4.2.3.
func newInitCmd(global *GlobalFlags, stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var targetNode string
	var force bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a local dnsApp.config from a Technitium server (or a blank template)",
		Long: `Resolves Technitium credentials (environment variables or secrets.json), fetches the
current Advanced Blocking configuration from the server, previews it, and asks for
explicit confirmation before writing it to disk as dnsApp.config.

If no credentials can be resolved, init prompts for them interactively (unless
--json is set, in which case it errors clearly instead of prompting). If the
fetched server configuration is empty (a fresh Technitium instance with nothing
configured yet), init falls back to a minimal template. If two or more nodes are
configured, init also checks that they agree on their current configuration before
proceeding, and never auto-merges disagreeing nodes.`,
		// SilenceUsage: without it, cobra appends a full Usage:/Flags: block to
		// stderr on any returned error, which is noisy for --json's scriptable
		// contract (matches the same reasoning documented on the verify command).
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(stdin)

			// 1. Resolve credentials.
			nodes, counts, err := resolveNodesWithSources()
			if err != nil {
				return err
			}

			if len(nodes) == 0 {
				if jsonOutput {
					return errors.New("no Technitium credentials resolvable: set TECHNITIUM_URL/TECHNITIUM_TOKEN or define ~/.config/pab/secrets.json (--json mode does not prompt interactively)")
				}
				nodes, err = promptForCredentials(reader, stdout, force)
				if err != nil {
					return err
				}
				if len(nodes) == 0 {
					return errors.New("no credentials entered; aborting init")
				}
			} else if !jsonOutput {
				fmt.Fprintf(stdout, "Resolved %d node(s): %d from environment variables, %d from secrets.json.\n", len(nodes), counts.Environment, counts.SecretsFile)
			}

			// 2. Fetch (with a bounded loop-back to credential re-entry on failure).
			var selectedName string
			var cfg *config.Config

			for attempt := 1; ; attempt++ {
				var nodeNames []string
				for name := range nodes {
					nodeNames = append(nodeNames, name)
				}
				slices.Sort(nodeNames)

				selectedName = targetNode
				if selectedName == "" {
					selectedName = nodeNames[0]
				}
				selectedNode, ok := nodes[selectedName]
				if !ok {
					return fmt.Errorf("target node %q is not defined in the resolved credentials", selectedName)
				}

				cfg, err = fetchOrTemplate(stdout, selectedName, selectedNode, jsonOutput)
				if err == nil {
					break
				}

				fmt.Fprintf(stderr, "Error: %v\n", err)

				if jsonOutput || attempt >= maxCredentialAttempts {
					return err
				}

				retry, promptErr := promptLine(reader, stdout, "Re-enter credentials and try again? [y/N]: ")
				if promptErr != nil {
					return promptErr
				}
				if !isYes(retry) {
					return err
				}

				nodes, err = promptForCredentials(reader, stdout, force)
				if err != nil {
					return err
				}
				if len(nodes) == 0 {
					return errors.New("no credentials entered; aborting init")
				}
				// A fresh credential entry may use different node names than the
				// previous --node selection, so let step 2 pick again.
				targetNode = ""
			}

			// 3. [Only when 2+ nodes are configured] Fleet-consistency check.
			//
			// All diagnostic output from this step goes to stderr when --json is
			// set: stdout is a documented clean-JSON contract (matching verify.go's
			// same reasoning), and disagreement/progress text has no business
			// appearing there just because a fleet happens to have 2+ nodes.
			if len(nodes) >= 2 {
				diagWriter := stdout
				if jsonOutput {
					diagWriter = stderr
				}

				agree, err := checkFleetConsistency(diagWriter, nodes)
				if err != nil {
					return fmt.Errorf("fleet consistency check failed: %w", err)
				}
				if !agree {
					if targetNode == "" {
						return fmt.Errorf("configured nodes disagree on their current configuration; re-run with --node <name> to explicitly select one as the source of truth (pab never auto-merges disagreeing nodes)")
					}
					fmt.Fprintf(diagWriter, "Proceeding with node %q as the explicitly selected source of truth.\n", selectedName)
				} else if !jsonOutput {
					fmt.Fprintf(stdout, "All %d configured nodes agree on their current configuration.\n", len(nodes))
				}
			}

			// 4. Preview.
			if jsonOutput {
				out, err := json.MarshalIndent(cfg, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to format configuration as JSON: %w", err)
				}
				fmt.Fprintln(stdout, string(out))
				return nil
			}

			printConfigPreview(stdout, cfg)

			// 5. Explicit commit.
			return commitConfig(reader, stdout, global.ConfigFile, cfg, force)
		},
	}

	cmd.Flags().StringVar(&targetNode, "node", "", "Node to fetch from (and, if configured nodes disagree, the explicit source of truth)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Write the resolved configuration without an interactive confirmation prompt")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print the resolved configuration as JSON and exit without writing or prompting")

	return cmd
}

// promptForCredentials interactively collects Technitium credentials when none
// could be resolved from the environment or secrets.json. For a single node it
// prints shell export lines for the user to add to their own profile (pab cannot
// persist environment variables for the user's shell); for two or more nodes it
// writes ~/.config/pab/secrets.json (chmod 600, set directly at creation) using the
// NodeConfig.Name field so each node has a stable identity.
//
// If secrets.json already exists, its existing node names are listed and an
// explicit confirmation (or --force) is required before it is overwritten --
// this mirrors commitConfig's diff-before-overwrite treatment of dnsApp.config;
// secrets.json deserves at least the same care since it's the more sensitive of
// the two files and this is reachable both on first run and from init's
// fetch-failure retry loop, where an existing multi-node secrets.json could
// otherwise be silently replaced by whatever the user types next.
func promptForCredentials(reader *bufio.Reader, stdout io.Writer, force bool) (map[string]NodeConfig, error) {
	fmt.Fprintln(stdout, "No Technitium credentials found via environment variables or secrets.json.")

	url, err := promptLine(reader, stdout, "Technitium server URL (e.g. https://dns.example.com:5380): ")
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, errors.New("a server URL is required")
	}

	token, err := promptLine(reader, stdout, "API token: ")
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("an API token is required")
	}

	multi, err := promptLine(reader, stdout, "Do you manage more than one Technitium node? [y/N]: ")
	if err != nil {
		return nil, err
	}

	if !isYes(multi) {
		fmt.Fprintln(stdout, "\nSingle-node setup. pab cannot persist environment variables for you;")
		fmt.Fprintln(stdout, "add the following to your shell profile (e.g. ~/.bashrc, ~/.zshrc):")
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "  export TECHNITIUM_URL=%q\n", url)
		fmt.Fprintf(stdout, "  export TECHNITIUM_TOKEN=%q\n", token)
		fmt.Fprintln(stdout)
		return map[string]NodeConfig{"default": {URL: url, Token: token}}, nil
	}

	name, err := promptLine(reader, stdout, "Name for this node (e.g. dns1, prod): ")
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = "node-0"
	}

	sc := SecretsConfig{Nodes: []NodeConfig{{Name: name, URL: url, Token: token}}}

	for {
		another, err := promptLine(reader, stdout, "Add another node? [y/N]: ")
		if err != nil {
			return nil, err
		}
		if !isYes(another) {
			break
		}

		nURL, err := promptLine(reader, stdout, "  Node URL: ")
		if err != nil {
			return nil, err
		}
		nToken, err := promptLine(reader, stdout, "  API token: ")
		if err != nil {
			return nil, err
		}
		nName, err := promptLine(reader, stdout, "  Node name: ")
		if err != nil {
			return nil, err
		}
		if nName == "" {
			nName = fmt.Sprintf("node-%d", len(sc.Nodes))
		}
		sc.Nodes = append(sc.Nodes, NodeConfig{Name: nName, URL: nURL, Token: nToken})
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user config directory: %w", err)
	}
	secretsPath := filepath.Join(configDir, "pab", "secrets.json")

	if !force {
		if existingBytes, statErr := os.ReadFile(secretsPath); statErr == nil {
			var existing SecretsConfig
			if jsonErr := json.Unmarshal(existingBytes, &existing); jsonErr == nil {
				names := make([]string, 0, len(existing.Nodes))
				for i, n := range existing.Nodes {
					name := n.Name
					if name == "" {
						name = fmt.Sprintf("node-%d", i)
					}
					names = append(names, name)
				}
				fmt.Fprintf(stdout, "\n%s already has %d node(s): %s\n", secretsPath, len(names), strings.Join(names, ", "))
			} else {
				fmt.Fprintf(stdout, "\n%s already exists but could not be parsed to list its nodes.\n", secretsPath)
			}

			confirm, promptErr := promptLine(reader, stdout, fmt.Sprintf("Overwrite %s with the %d node(s) just entered? [y/N]: ", secretsPath, len(sc.Nodes)))
			if promptErr != nil {
				return nil, promptErr
			}
			if !isYes(confirm) {
				return nil, fmt.Errorf("not overwriting existing %s; re-run with --force to skip this confirmation, or edit the file manually", secretsPath)
			}
		}
	}

	if err := writeSecretsFile(secretsPath, sc); err != nil {
		return nil, fmt.Errorf("failed to write secrets.json: %w", err)
	}
	fmt.Fprintf(stdout, "\nWrote %d node(s) to %s (chmod 600).\n", len(sc.Nodes), secretsPath)

	nodes := make(map[string]NodeConfig)
	for _, n := range sc.Nodes {
		nodes[n.Name] = n
	}
	return nodes, nil
}

// promptLine writes prompt to stdout and reads a single trimmed line from reader.
// A missing trailing newline (io.EOF) is not treated as an error so a scripted
// stdin without a final newline still yields whatever text was read.
func promptLine(reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	fmt.Fprint(stdout, prompt)
	text, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(text), nil
}

// isYes reports whether a trimmed prompt response means "yes".
func isYes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}

// fetchOrTemplate fetches the current Advanced Blocking config from the given node
// via client.GetAppConfig(), which is already parseable via config.Config's existing
// UnmarshalJSON. An empty result (a fresh Technitium instance) falls back to the
// minimal template shape shipped in dnsApp.config.example.
func fetchOrTemplate(stdout io.Writer, nodeName string, node NodeConfig, jsonOutput bool) (*config.Config, error) {
	if !jsonOutput {
		fmt.Fprintf(stdout, "Fetching configuration from node %q (%s)...\n", nodeName, node.URL)
	}

	c := client.NewClient(node.URL, node.Token)
	raw, err := c.GetAppConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch configuration from node %q: %w", nodeName, err)
	}

	if raw == "" {
		if !jsonOutput {
			fmt.Fprintln(stdout, "No existing Advanced Blocking config found on that server -- starting from a blank template.")
		}
		return templateConfig(), nil
	}

	var cfg config.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration fetched from node %q: %w", nodeName, err)
	}
	return &cfg, nil
}

// templateConfig returns a minimal valid configuration used as the local fallback
// when a Technitium server has no existing Advanced Blocking config yet. It mirrors
// the shape shipped in dnsApp.config.example at the repo root, built directly from
// config.Config/config.Group's real fields (not the maintainer's personal config).
func templateConfig() *config.Config {
	return &config.Config{
		EnableBlocking:                    true,
		BlockingAnswerTtl:                 60,
		BlockListUrlUpdateIntervalHours:   24,
		BlockListUrlUpdateIntervalMinutes: 0,
		NetworkGroupMap: map[string]string{
			"0.0.0.0/0": "Default",
		},
		Groups: []config.Group{
			{
				Name:                   "Default",
				EnableBlocking:         true,
				AllowTxtBlockingReport: false,
				BlockAsNxDomain:        false,
			},
		},
	}
}

// checkFleetConsistency fetches the current configuration from every resolved node
// and pairwise-compares them using printStructuralDiff -- the same diff primitive
// `deploy --dry-run` uses, called once per node pair instead of once per deploy
// node. Disagreements are printed immediately, labeled by node name; pab never
// auto-merges disagreeing nodes. It returns whether all nodes agree.
func checkFleetConsistency(w io.Writer, nodes map[string]NodeConfig) (bool, error) {
	var names []string
	for name := range nodes {
		names = append(names, name)
	}
	slices.Sort(names)

	type fetchedNode struct {
		name string
		cfg  *config.Config
	}

	fetched := make([]fetchedNode, 0, len(names))
	for _, name := range names {
		node := nodes[name]
		c := client.NewClient(node.URL, node.Token)
		raw, err := c.GetAppConfig()
		if err != nil {
			return false, fmt.Errorf("failed to fetch configuration from node %q: %w", name, err)
		}
		var cfg config.Config
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return false, fmt.Errorf("failed to parse configuration from node %q: %w", name, err)
			}
		}
		fetched = append(fetched, fetchedNode{name: name, cfg: &cfg})
	}

	agree := true
	var warnings bytes.Buffer
	for i := 0; i < len(fetched); i++ {
		for j := i + 1; j < len(fetched); j++ {
			var diff bytes.Buffer
			pairLabel := fmt.Sprintf("%s vs %s", fetched[i].name, fetched[j].name)
			printStructuralDiff(&diff, pairLabel, fetched[i].cfg, fetched[j].cfg)
			if hasStructuralDiffContent(diff.String()) {
				agree = false
				fmt.Fprintf(&warnings, "%q and %q disagree:\n", fetched[i].name, fetched[j].name)
				warnings.Write(diff.Bytes())
			}
		}
	}

	if !agree {
		fmt.Fprintln(w, "WARNING: configured nodes disagree on their current configuration:")
		fmt.Fprintln(w)
		w.Write(warnings.Bytes())
	}

	return agree, nil
}

// hasStructuralDiffContent reports whether printStructuralDiff's output contains
// any actual diff lines, as opposed to just its header and trailing blank line.
func hasStructuralDiffContent(diffOutput string) bool {
	for _, line := range strings.Split(diffOutput, "\n") {
		if strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

// printConfigPreview prints a structural summary of a resolved configuration to
// stdout, clearly labeled as not yet saved (mirroring the summary style
// printStructuralDiff already uses for groups/mappings).
func printConfigPreview(w io.Writer, cfg *config.Config) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Preview (NOT YET SAVED):")
	fmt.Fprintf(w, "  Blocking enabled: %t\n", cfg.EnableBlocking)
	fmt.Fprintf(w, "  Blocking answer TTL: %d\n", cfg.BlockingAnswerTtl)
	fmt.Fprintf(w, "  Groups (%d):\n", len(cfg.Groups))

	names := make([]string, 0, len(cfg.Groups))
	for _, g := range cfg.Groups {
		names = append(names, g.Name)
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(w, "    - %s\n", name)
	}

	fmt.Fprintf(w, "  Network client mappings: %d\n", len(cfg.NetworkGroupMap))
	fmt.Fprintln(w)
}

// commitConfig implements step 5's explicit-commit flow: never silently clobber an
// existing local dnsApp.config, and never write without either an explicit
// confirmation or --force.
func commitConfig(reader *bufio.Reader, stdout io.Writer, path string, cfg *config.Config, force bool) error {
	if _, statErr := os.Stat(path); statErr == nil {
		existing, err := loadConfig(path)
		if err == nil {
			var diff bytes.Buffer
			printStructuralDiff(&diff, path, existing, cfg)
			if hasStructuralDiffContent(diff.String()) {
				fmt.Fprintf(stdout, "%q already exists and differs from the fetched/template configuration:\n\n", path)
				stdout.Write(diff.Bytes())
			} else {
				fmt.Fprintf(stdout, "%q already exists and matches the fetched/template configuration.\n", path)
			}
		} else {
			fmt.Fprintf(stdout, "%q already exists but could not be parsed for comparison: %v\n", path, err)
		}
	}

	if !force {
		answer, err := promptLine(reader, stdout, fmt.Sprintf("Save this configuration as %q? [y/N]: ", path))
		if err != nil {
			return err
		}
		if !isYes(answer) {
			fmt.Fprintln(stdout, "Not saved.")
			return nil
		}
	}

	// Validate before writing, matching map/unmap's existing pattern -- a
	// structurally-fetched-but-semantically-invalid remote config (e.g. a
	// network mapping referencing a since-renamed group) should fail loudly
	// here, not silently land on disk and only surface on the next map/deploy.
	if err := config.ValidateConfig(cfg); err != nil {
		return fmt.Errorf("fetched/template configuration failed validation: %w", err)
	}

	if err := saveConfig(path, cfg); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Saved configuration to %q\n", path)
	return nil
}
