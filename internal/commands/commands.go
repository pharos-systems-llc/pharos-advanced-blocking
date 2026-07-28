package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/iamrichardd/pharos-advanced-blocking/internal/client"
	"github.com/iamrichardd/pharos-advanced-blocking/internal/config"
	"github.com/iamrichardd/pharos-advanced-blocking/internal/plugin"
	"github.com/spf13/cobra"
)

// SecretsConfig defines the structure for local secrets file ~/.config/pab/secrets.json.
// This file is the required mechanism for configuring two or more Technitium nodes;
// environment variables (TECHNITIUM_URL/TECHNITIUM_TOKEN) only support a single node.
type SecretsConfig struct {
	Nodes []NodeConfig `json:"nodes"`
}

// NodeConfig defines the URL and token for a Technitium node.
type NodeConfig struct {
	// Name optionally identifies the node (e.g. "prod", "dns1"). When omitted,
	// resolveNodes falls back to a positional "node-<index>" name.
	Name  string `json:"name,omitempty"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

// GlobalFlags stores global CLI flag values.
type GlobalFlags struct {
	ConfigFile string
}

// NewRootCmd constructs and registers all CLI commands.
// It allows passing stdin, stdout, and stderr for flexible CLI testing.
func NewRootCmd(stdin io.Reader, stdout, stderr io.Writer, version, commit, date string) *cobra.Command {
	global := &GlobalFlags{}

	rootCmd := &cobra.Command{
		Use:     "pab",
		Short:   "Pharos Advanced Blocking CLI",
		Long:    `A command-line interface to manage, validate, and sync Technitium Advanced Blocking configurations.`,
		Version: fmt.Sprintf("%s, commit %s, built at %s", version, commit, date),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Strict Credentials Guard check on startup.
			// Abort immediately with a high-priority warning if permissions are weaker than 0600.
			configDir, err := os.UserConfigDir()
			if err == nil {
				secretsPath := filepath.Join(configDir, "pab", "secrets.json")
				err = config.VerifyCredentialsFile(secretsPath)
				if err != nil {
					if errors.Is(err, config.ErrWeakerPermissions) {
						// Print warning and return error to abort command run
						fmt.Fprintf(stderr, "SECURITY WARNING: %v\n", err)
						return err
					}
					// ErrCredentialsNotFound is ok, we fall back to environment variables
				}
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(stdout, "Welcome to Pharos Advanced Blocking (pab)!")
		},
	}

	rootCmd.SetIn(stdin)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.PersistentFlags().StringVarP(&global.ConfigFile, "config", "c", "dnsApp.config", "Path to Advanced Blocking configuration file")

	// Register subcommands
	rootCmd.AddCommand(newMapCmd(global, stdout, stderr))
	rootCmd.AddCommand(newUnmapCmd(global, stdout, stderr))
	rootCmd.AddCommand(newDeployCmd(global, stdin, stdout, stderr))
	rootCmd.AddCommand(newListNodesCmd(global, stdout, stderr))
	rootCmd.AddCommand(newInitCmd(global, stdin, stdout, stderr))
	rootCmd.AddCommand(newVerifyCmd(global, stdout, stderr))

	// Load Plugins
	configDir, err := os.UserConfigDir()
	if err == nil {
		pluginDir := filepath.Join(configDir, "pab", "plugins")
		manager := plugin.NewManager([]string{pluginDir, "./plugins"})
		// Best-effort loading, ignore errors
		_ = manager.LoadPlugins()
		for _, p := range manager.Plugins {
			for _, pCmd := range p.RegisterCommands() {
				// Set output streams for plugin commands if applicable
				pCmd.SetIn(stdin)
				pCmd.SetOut(stdout)
				pCmd.SetErr(stderr)
				rootCmd.AddCommand(pCmd)
			}
		}
	}

	return rootCmd
}

// newMapCmd defines the "pab map --ip <IP> --group <GROUP>" command.
func newMapCmd(global *GlobalFlags, stdout, stderr io.Writer) *cobra.Command {
	var ip string
	var group string

	cmd := &cobra.Command{
		Use:   "map",
		Short: "Map/reassign a client IP to a blocking group",
		Long:  `Adds or updates a client IP mapping in the configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ip == "" {
				return errors.New("missing required flag: --ip")
			}
			if group == "" {
				return errors.New("missing required flag: --group")
			}

			// Load existing configuration from disk
			cfg, err := loadConfig(global.ConfigFile)
			if err != nil {
				return err
			}

			// Initialize NetworkGroupMap if nil
			if cfg.NetworkGroupMap == nil {
				cfg.NetworkGroupMap = make(map[string]string)
			}

			// Update the client IP mapping
			cfg.NetworkGroupMap[ip] = group

			// Run validation engine on the updated configuration
			if err := config.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			// Save back to disk
			if err := saveConfig(global.ConfigFile, cfg); err != nil {
				return err
			}

			fmt.Fprintf(stdout, "Successfully mapped IP %s to group %q\n", ip, group)
			return nil
		},
	}

	cmd.Flags().StringVar(&ip, "ip", "", "Client IP or CIDR range")
	cmd.Flags().StringVar(&group, "group", "", "Target blocking group name")

	return cmd
}

// newUnmapCmd defines the "pab unmap --ip <IP>" command.
func newUnmapCmd(global *GlobalFlags, stdout, stderr io.Writer) *cobra.Command {
	var ip string

	cmd := &cobra.Command{
		Use:   "unmap",
		Short: "Delete a client IP mapping from the configuration",
		Long:  `Deletes a client IP mapping from the configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ip == "" {
				return errors.New("missing required flag: --ip")
			}

			cfg, err := loadConfig(global.ConfigFile)
			if err != nil {
				return err
			}

			if cfg.NetworkGroupMap == nil {
				return fmt.Errorf("client IP %s is not mapped (no network group map exists)", ip)
			}

			// Check if mapping exists
			if _, exists := cfg.NetworkGroupMap[ip]; !exists {
				return fmt.Errorf("client IP %s is not mapped in the configuration", ip)
			}

			// Remove mapping
			delete(cfg.NetworkGroupMap, ip)

			// Run validation
			if err := config.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("configuration validation failed: %w", err)
			}

			// Save config
			if err := saveConfig(global.ConfigFile, cfg); err != nil {
				return err
			}

			fmt.Fprintf(stdout, "Successfully unmapped IP %s\n", ip)
			return nil
		},
	}

	cmd.Flags().StringVar(&ip, "ip", "", "Client IP or CIDR range")

	return cmd
}

// newDeployCmd defines the "pab deploy [--dry-run] [-f|--force] [--node <name>]" command.
func newDeployCmd(global *GlobalFlags, stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var dryRun bool
	var force bool
	var targetNode string

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Sync configuration to Technitium server API nodes",
		Long:  `Syncs the validated configuration to Technitium server API nodes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load and validate local configuration
			cfg, err := loadConfig(global.ConfigFile)
			if err != nil {
				return err
			}

			if err := config.ValidateConfig(cfg); err != nil {
				return fmt.Errorf("local configuration is invalid: %w", err)
			}

			// Marshal configuration to JSON
			configJSONBytes, err := json.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal local configuration: %w", err)
			}
			configJSON := string(configJSONBytes)

			// Resolve Technitium target nodes and credentials
			nodes, err := resolveNodes()
			if err != nil {
				return err
			}

			if len(nodes) == 0 {
				return errors.New("no Technitium nodes configured. Set environment variables or define them in ~/.config/pab/secrets.json")
			}

			// If target node is specified, verify it exists and narrow down targets
			targets := make(map[string]NodeConfig)
			if targetNode != "" {
				node, exists := nodes[targetNode]
				if !exists {
					return fmt.Errorf("target node %q is not defined in the configuration or environment", targetNode)
				}
				targets[targetNode] = node
			} else {
				targets = nodes
			}

			// If not dry-run and not force, ask for confirmation once before processing all nodes
			if !dryRun && !force {
				fmt.Fprintf(stdout, "Deploy to all nodes? (y/N): ")
				reader := bufio.NewReader(stdin)
				text, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				text = strings.ToLower(strings.TrimSpace(text))
				if text != "y" && text != "yes" {
					fmt.Fprintf(stdout, "Deployment cancelled.\n")
					return nil
				}
			}

			// Process nodes
			for name, node := range targets {
				c := client.NewClient(node.URL, node.Token)

				if dryRun {
					fmt.Fprintf(stdout, "Checking configuration on node %q...\n", name)
					remoteConfigJSON, err := c.GetAppConfig()
					if err != nil {
						fmt.Fprintf(stderr, "Warning: failed to fetch remote configuration for dry-run comparison: %v\n", err)
						continue
					}

					var remoteCfg config.Config
					if remoteConfigJSON != "" {
						_ = json.Unmarshal([]byte(remoteConfigJSON), &remoteCfg)
					}

					// Print structural diff
					printStructuralDiff(stdout, name, &remoteCfg, cfg)
					continue
				}

				fmt.Fprintf(stdout, "Syncing configuration to node %q...\n", name)
				if err := c.SetAppConfig(configJSON); err != nil {
					return fmt.Errorf("failed to deploy to node %q: %w", name, err)
				}
				fmt.Fprintf(stdout, "Successfully deployed configuration to node %q\n", name)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Runs validation check and structural diff without writing to API")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force deployment without confirmation")
	cmd.Flags().StringVar(&targetNode, "node", "", "Override target to a specific node name")

	return cmd
}

// newListNodesCmd defines the "pab list-nodes" command.
func newListNodesCmd(global *GlobalFlags, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-nodes",
		Short: "List configured Technitium nodes",
		Long:  `Display all configured Technitium nodes from environment variables and secrets.json with their URLs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve Technitium nodes from environment and secrets.json
			nodes, counts, err := resolveNodesWithSources()
			if err != nil {
				return err
			}

			if len(nodes) == 0 {
				fmt.Fprintf(stdout, "No Technitium nodes configured. Set environment variables or define them in ~/.config/pab/secrets.json\n")
				return nil
			}

			// Make a multi-source merge visible instead of blending silently.
			if counts.Environment > 0 && counts.SecretsFile > 0 {
				fmt.Fprintf(stdout, "Found %d node(s) from environment variables + %d from secrets.json = %d total\n\n", counts.Environment, counts.SecretsFile, len(nodes))
			}

			// Sort node names for consistent output
			var nodeNames []string
			for name := range nodes {
				nodeNames = append(nodeNames, name)
			}
			slices.Sort(nodeNames)

			// Print header
			fmt.Fprintf(stdout, "%-20s %s\n", "Node Name", "URL")
			fmt.Fprintf(stdout, "%-20s %s\n", strings.Repeat("-", 9), strings.Repeat("-", 45))

			// Print nodes
			for _, name := range nodeNames {
				node := nodes[name]
				fmt.Fprintf(stdout, "%-20s %s\n", name, node.URL)
			}

			return nil
		},
	}

	return cmd
}

// loadConfig reads the config file from disk.
func loadConfig(path string) (*config.Config, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuration file %q not found. Run 'pab init' to bootstrap one from a Technitium server (or a blank template), or verify the file path", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.Config
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from config file: %w", err)
	}

	return &cfg, nil
}

// saveConfig writes the config file to disk with pretty-print.
func saveConfig(path string, cfg *config.Config) error {
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format config: %w", err)
	}

	if err := os.WriteFile(path, bytes, 0644); err != nil {
		return fmt.Errorf("failed to write config to file: %w", err)
	}

	return nil
}

// NodeSourceCounts summarizes how many resolved nodes came from each credential
// source, so callers can surface a visible merge (e.g. "1 from environment
// variables + 2 from secrets.json = 3 total") instead of blending silently.
type NodeSourceCounts struct {
	Environment int
	SecretsFile int
}

// resolveNodes discovers Technitium node configurations from environment variables
// and secrets.json. It is a thin wrapper around resolveNodesWithSources for callers
// that don't need source visibility.
func resolveNodes() (map[string]NodeConfig, error) {
	nodes, _, err := resolveNodesWithSources()
	return nodes, err
}

// resolveNodesWithSources discovers Technitium node configurations from environment
// variables and secrets.json, and reports how many nodes were contributed by each
// source.
//
// Target design: environment variables (TECHNITIUM_URL/TECHNITIUM_TOKEN) support
// exactly one node, registered as "default". Two or more nodes require
// ~/.config/pab/secrets.json.
func resolveNodesWithSources() (map[string]NodeConfig, NodeSourceCounts, error) {
	nodes := make(map[string]NodeConfig)
	var counts NodeSourceCounts

	// 1. Single-node environment variable fallback: TECHNITIUM_URL / TECHNITIUM_TOKEN -> "default".
	if defaultURL := os.Getenv("TECHNITIUM_URL"); defaultURL != "" {
		if defaultToken := os.Getenv("TECHNITIUM_TOKEN"); defaultToken != "" {
			nodes["default"] = NodeConfig{
				URL:   defaultURL,
				Token: defaultToken,
			}
			counts.Environment++
		}
	}

	// 2. Read ~/.config/pab/secrets.json (required for 2+ nodes).
	configDir, err := os.UserConfigDir()
	if err == nil {
		secretsPath := filepath.Join(configDir, "pab", "secrets.json")
		if _, statErr := os.Stat(secretsPath); statErr == nil {
			// Permission is verified in PersistentPreRunE, so we just read here.
			// A malformed/unreadable file must surface as an error rather than
			// silently behaving like "no secrets.json" -- otherwise a typo can
			// look identical to zero configured nodes to every caller.
			fileBytes, readErr := os.ReadFile(secretsPath)
			if readErr != nil {
				return nil, counts, fmt.Errorf("failed to read secrets file %q: %w", secretsPath, readErr)
			}
			var sc SecretsConfig
			if jsonErr := json.Unmarshal(fileBytes, &sc); jsonErr != nil {
				return nil, counts, fmt.Errorf("failed to parse secrets file %q: %w", secretsPath, jsonErr)
			}
			for i, node := range sc.Nodes {
				name := node.Name
				if name == "" {
					name = fmt.Sprintf("node-%d", i)
				}
				nodes[name] = node
				counts.SecretsFile++
			}
		}
	}

	return nodes, counts, nil
}

// writeSecretsFile marshals a SecretsConfig to JSON and writes it to path with
// strict 0600 permissions set directly at creation (via config.WriteSecretsFile).
// It is the single write path used by `pab init` (and any future command) to
// persist ~/.config/pab/secrets.json consistently.
func writeSecretsFile(path string, sc SecretsConfig) error {
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format secrets config: %w", err)
	}

	if err := config.WriteSecretsFile(path, data); err != nil {
		return err
	}

	return nil
}

// printStructuralDiff renders a simplified, human-readable structural diff.
func printStructuralDiff(w io.Writer, nodeName string, remote, local *config.Config) {
	fmt.Fprintf(w, "Structural configuration diff for node %q:\n", nodeName)

	if remote.EnableBlocking != local.EnableBlocking {
		fmt.Fprintf(w, "  ~ EnableBlocking: %t -> %t\n", remote.EnableBlocking, local.EnableBlocking)
	}
	if remote.BlockingAnswerTtl != local.BlockingAnswerTtl {
		fmt.Fprintf(w, "  ~ BlockingAnswerTtl: %d -> %d\n", remote.BlockingAnswerTtl, local.BlockingAnswerTtl)
	}

	// Diff Groups
	remoteGroups := make(map[string]config.Group)
	for _, g := range remote.Groups {
		remoteGroups[g.Name] = g
	}

	localGroups := make(map[string]config.Group)
	for _, g := range local.Groups {
		localGroups[g.Name] = g
	}

	// Sorted list of all group names
	allGroupNames := make(map[string]bool)
	for k := range remoteGroups {
		allGroupNames[k] = true
	}
	for k := range localGroups {
		allGroupNames[k] = true
	}

	var groupNames []string
	for k := range allGroupNames {
		groupNames = append(groupNames, k)
	}
	slices.Sort(groupNames)

	for _, gName := range groupNames {
		rg, inRemote := remoteGroups[gName]
		lg, inLocal := localGroups[gName]

		if !inRemote {
			fmt.Fprintf(w, "  + Group %q (Added)\n", gName)
		} else if !inLocal {
			fmt.Fprintf(w, "  - Group %q (Removed)\n", gName)
		} else {
			// Check if modified (simplified check)
			if rg.EnableBlocking != lg.EnableBlocking || !reflect.DeepEqual(rg.Blocked, lg.Blocked) || !reflect.DeepEqual(rg.Allowed, lg.Allowed) {
				fmt.Fprintf(w, "  ~ Group %q (Modified)\n", gName)
			}
		}
	}

	// Diff NetworkGroupMap
	allIPs := make(map[string]bool)
	for k := range remote.NetworkGroupMap {
		allIPs[k] = true
	}
	for k := range local.NetworkGroupMap {
		allIPs[k] = true
	}

	var ips []string
	for k := range allIPs {
		ips = append(ips, k)
	}
	slices.Sort(ips)

	hasMapDiff := false
	for _, ip := range ips {
		rv, inRemote := remote.NetworkGroupMap[ip]
		lv, inLocal := local.NetworkGroupMap[ip]

		if !inRemote {
			if !hasMapDiff {
				fmt.Fprintln(w, "  Client Mappings:")
				hasMapDiff = true
			}
			fmt.Fprintf(w, "    + %s -> %s\n", ip, lv)
		} else if !inLocal {
			if !hasMapDiff {
				fmt.Fprintln(w, "  Client Mappings:")
				hasMapDiff = true
			}
			fmt.Fprintf(w, "    - %s -> %s\n", ip, rv)
		} else if rv != lv {
			if !hasMapDiff {
				fmt.Fprintln(w, "  Client Mappings:")
				hasMapDiff = true
			}
			fmt.Fprintf(w, "    ~ %s: %s -> %s\n", ip, rv, lv)
		}
	}
	fmt.Fprintln(w)
}
