package main

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/commands"
	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/config"
	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/tui"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	// Check if running with no arguments
	if len(os.Args) == 1 {
		if isatty.IsTerminal(os.Stdin.Fd()) {
			// TTY detected: launch TUI
			if err := launchTUI("dnsApp.config"); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		// Non-TTY with no args: show helpful error
		fmt.Fprintln(os.Stderr, "Error: Interactive mode requires a terminal.")
		fmt.Fprintln(os.Stderr, "For automated use, specify a subcommand:")
		fmt.Fprintln(os.Stderr, "  pab map --ip <IP> --group <GROUP>")
		fmt.Fprintln(os.Stderr, "  pab deploy -f")
		fmt.Fprintln(os.Stderr, "Or use --json for machine-readable output.")
		fmt.Fprintln(os.Stderr, "Or use 'pab --help' to see all commands.")
		os.Exit(1)
	}

	// Arguments provided: run normal Cobra dispatch
	cmd := commands.NewRootCmd(os.Stdin, os.Stdout, os.Stderr, Version, Commit, Date)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// launchTUI loads the configuration and starts the interactive TUI.
func launchTUI(configPath string) error {
	// Load configuration file
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration file %q not found. Run 'pab init' to bootstrap one from a Technitium server (or a blank template), or verify the file path", configPath)
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON configuration
	var cfg config.Config
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return fmt.Errorf("failed to parse JSON from config file: %w", err)
	}

	// Create TUI model with configuration
	m := tui.New(&cfg)

	// Run Bubble Tea program with full-screen rendering
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
