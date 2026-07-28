package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrichardd/pharos-advanced-blocking/internal/config"
)

// helperWriteConfig creates a mock config file.
func helperWriteConfig(t *testing.T, path string, cfg *config.Config) {
	t.Helper()
	bytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	err = os.WriteFile(path, bytes, 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

// helperReadConfig reads a config file.
func helperReadConfig(t *testing.T, path string) *config.Config {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	var cfg config.Config
	err = json.Unmarshal(bytes, &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	return &cfg
}

func TestMapCommand(t *testing.T) {
	t.Run("successfully maps an IP to an existing group", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		initialConfig := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
			NetworkGroupMap: map[string]string{
				"192.168.1.10": "Default",
			},
		}
		helperWriteConfig(t, configPath, initialConfig)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "map", "--ip", "192.168.1.20", "--group", "Default"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing map command: %v", err)
		}

		// Verify output
		if !strings.Contains(outBuf.String(), "Successfully mapped IP 192.168.1.20 to group \"Default\"") {
			t.Errorf("unexpected output: %s", outBuf.String())
		}

		// Verify on-disk config updated
		cfg := helperReadConfig(t, configPath)
		if cfg.NetworkGroupMap["192.168.1.20"] != "Default" {
			t.Errorf("expected 192.168.1.20 to be mapped to Default, got %s", cfg.NetworkGroupMap["192.168.1.20"])
		}
	})

	t.Run("fails when mapping to a non-existent group", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		initialConfig := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, initialConfig)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "map", "--ip", "192.168.1.20", "--group", "NonExistent"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error due to invalid group name, got nil")
		}
		if !strings.Contains(err.Error(), "target group \"NonExistent\" does not exist") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("fails when mapping an invalid IP address", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		initialConfig := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, initialConfig)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "map", "--ip", "invalid-ip", "--group", "Default"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error due to invalid IP format, got nil")
		}
		if !strings.Contains(err.Error(), "must be a valid IPv4 or IPv6 address") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("fails when missing required flags", func(t *testing.T) {
		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"map", "--ip", "192.168.1.10"}) // missing --group

		err := rootCmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "missing required flag: --group") {
			t.Errorf("expected missing flag error, got: %v", err)
		}
	})
}

func TestUnmapCommand(t *testing.T) {
	t.Run("successfully removes an existing mapping", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		initialConfig := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
			NetworkGroupMap: map[string]string{
				"192.168.1.10": "Default",
			},
		}
		helperWriteConfig(t, configPath, initialConfig)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "unmap", "--ip", "192.168.1.10"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing unmap: %v", err)
		}

		if !strings.Contains(outBuf.String(), "Successfully unmapped IP 192.168.1.10") {
			t.Errorf("unexpected output: %s", outBuf.String())
		}

		cfg := helperReadConfig(t, configPath)
		if _, exists := cfg.NetworkGroupMap["192.168.1.10"]; exists {
			t.Fatal("expected 192.168.1.10 to be removed from NetworkGroupMap")
		}
	})

	t.Run("fails when IP is not mapped", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		initialConfig := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
			NetworkGroupMap: map[string]string{
				"192.168.1.10": "Default",
			},
		}
		helperWriteConfig(t, configPath, initialConfig)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "unmap", "--ip", "192.168.1.99"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error unmapping non-existent IP, got nil")
		}
		if !strings.Contains(err.Error(), "client IP 192.168.1.99 is not mapped") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestDeployCommand(t *testing.T) {
	t.Run("successfully deploys configuration with -f flag", func(t *testing.T) {
		// Mock Technitium API Server
		serverCalled := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/apps/config/set" {
				serverCalled = true
				w.Write([]byte(`{"status": "ok"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}

		if !serverCalled {
			t.Error("expected mock server to be called during deployment")
		}
		if !strings.Contains(outBuf.String(), "Successfully deployed configuration to node") {
			t.Errorf("unexpected output: %s", outBuf.String())
		}
	})

	t.Run("successfully performs dry-run structural diff comparison", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				// Return a remote config with a deleted IP and a missing group
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"groups\":[{\"name\":\"Default\"},{\"name\":\"OldGroup\"}],\"networkGroupMap\":{\"192.168.1.15\":\"Default\"}}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		// Local config has NewGroup instead of OldGroup, and IP 192.168.1.20 instead of 192.168.1.15
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
				{Name: "NewGroup", EnableBlocking: true},
			},
			NetworkGroupMap: map[string]string{
				"192.168.1.20": "NewGroup",
			},
		}
		helperWriteConfig(t, configPath, cfg)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy", "--dry-run"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy dry-run error: %v", err)
		}

		output := outBuf.String()
		if !strings.Contains(output, `+ Group "NewGroup" (Added)`) {
			t.Errorf("missing added group output in diff: %s", output)
		}
		if !strings.Contains(output, `- Group "OldGroup" (Removed)`) {
			t.Errorf("missing removed group output in diff: %s", output)
		}
		if !strings.Contains(output, `+ 192.168.1.20 -> NewGroup`) {
			t.Errorf("missing added IP output in diff: %s", output)
		}
		if !strings.Contains(output, `- 192.168.1.15 -> Default`) {
			t.Errorf("missing removed IP output in diff: %s", output)
		}
	})

	t.Run("aborts deploy when confirmation is rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		t.Setenv("TECHNITIUM_URL", "http://localhost:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		// Pass 'n' to stdin to reject confirmation
		inBuf := bytes.NewBufferString("n\n")
		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(inBuf, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}

		if !strings.Contains(outBuf.String(), "Deployment cancelled.") {
			t.Errorf("unexpected output: %s", outBuf.String())
		}
	})

	t.Run("aborts deploy when secrets file has weaker permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Set XDG_CONFIG_HOME to check user config secrets file permission check
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		err := os.MkdirAll(pabConfigDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}

		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		// Write secrets file with weak permissions (0644 instead of 0600)
		err = os.WriteFile(secretsPath, []byte(`{"nodes":[{"url":"http://localhost:5380","token":"123"}]}`), 0644)
		if err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		// Explicitly chmod to ensure permissions are weaker
		_ = os.Chmod(secretsPath, 0644)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy"})

		err = rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error due to weak secrets file permissions, got nil")
		}

		if !errors.Is(err, config.ErrWeakerPermissions) {
			t.Errorf("expected ErrWeakerPermissions, got: %v", err)
		}
		if !strings.Contains(errBuf.String(), "SECURITY WARNING:") {
			t.Errorf("expected warning in stderr, got: %s", errBuf.String())
		}
	})

	t.Run("handles empty nodes array in secrets.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Set XDG_CONFIG_HOME to check user config secrets file
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		err := os.MkdirAll(pabConfigDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}

		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		// Write secrets file with empty nodes array
		err = os.WriteFile(secretsPath, []byte(`{"nodes":[]}`), 0600)
		if err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy"})

		err = rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error due to empty nodes array, got nil")
		}
		if !strings.Contains(err.Error(), "no Technitium nodes configured") {
			t.Errorf("expected error about no nodes, got: %v", err)
		}
	})

	t.Run("successfully deploys to single node in array", func(t *testing.T) {
		// Mock Technitium API Server
		serverCalled := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/apps/config/set" {
				serverCalled = true
				w.Write([]byte(`{"status": "ok"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Set XDG_CONFIG_HOME to check user config secrets file
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		err := os.MkdirAll(pabConfigDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}

		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		// Write secrets file with single node
		err = os.WriteFile(secretsPath, []byte(`{"nodes":[{"url":"`+server.URL+`","token":"mock-token"}]}`), 0600)
		if err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}

		if !serverCalled {
			t.Error("expected mock server to be called during deployment")
		}
		if !strings.Contains(outBuf.String(), "Successfully deployed configuration to node") {
			t.Errorf("unexpected output: %s", outBuf.String())
		}
	})

	t.Run("successfully deploys to multiple nodes with correct naming", func(t *testing.T) {
		// Mock Technitium API Server
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/apps/config/set" {
				callCount++
				w.Write([]byte(`{"status": "ok"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Set XDG_CONFIG_HOME to check user config secrets file
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		err := os.MkdirAll(pabConfigDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}

		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		// Write secrets file with multiple nodes
		err = os.WriteFile(secretsPath, []byte(`{"nodes":[{"url":"`+server.URL+`","token":"mock-token"},{"url":"`+server.URL+`","token":"mock-token"}]}`), 0600)
		if err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}

		// Should call the server at least twice, once for each node (may have additional validation calls)
		if callCount < 2 {
			t.Errorf("expected server to be called at least 2 times, got %d", callCount)
		}

		output := outBuf.String()
		// Verify correct node naming
		if !strings.Contains(output, "node-0") {
			t.Errorf("missing node-0 in output: %s", output)
		}
		if !strings.Contains(output, "node-1") {
			t.Errorf("missing node-1 in output: %s", output)
		}
	})

	t.Run("shows single confirmation prompt for multiple nodes", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Set XDG_CONFIG_HOME to check user config secrets file
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		err := os.MkdirAll(pabConfigDir, 0755)
		if err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}

		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		// Write secrets file with multiple nodes
		err = os.WriteFile(secretsPath, []byte(`{"nodes":[{"url":"http://localhost:5380","token":"123"},{"url":"http://localhost:5380","token":"123"}]}`), 0600)
		if err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		// Pass 'n' to stdin to reject confirmation
		inBuf := bytes.NewBufferString("n\n")
		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(inBuf, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}

		output := outBuf.String()
		// Verify single confirmation prompt appears
		if !strings.Contains(output, "Deploy to all nodes?") {
			t.Errorf("missing 'Deploy to all nodes?' confirmation in output: %s", output)
		}
		// Verify deployment was cancelled
		if !strings.Contains(output, "Deployment cancelled.") {
			t.Errorf("missing cancellation message in output: %s", output)
		}
	})

	t.Run("targets_specific_node_with_--node_flag", func(t *testing.T) {
		// Mock Technitium API Server
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/apps/config/set" {
				callCount++
				w.Write([]byte(`{"status": "ok"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Two or more nodes require secrets.json (env vars only support one node).
		// Use the new "name" field to give each node a stable identity.
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[` +
			`{"name":"dns1","url":"` + server.URL + `","token":"token1"},` +
			`{"name":"dns2","url":"` + server.URL + `","token":"token2"},` +
			`{"name":"dns3","url":"` + server.URL + `","token":"token3"}` +
			`]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy", "--node", "dns2", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}

		// Verify only one deployment call (for dns2)
		if callCount != 1 {
			t.Errorf("expected 1 server call for dns2, got %d", callCount)
		}

		output := outBuf.String()
		if !strings.Contains(output, "Syncing configuration to node \"dns2\"") {
			t.Errorf("expected sync message for dns2, got output: %s", output)
		}
		if !strings.Contains(output, "Successfully deployed configuration to node \"dns2\"") {
			t.Errorf("expected success message for dns2, got output: %s", output)
		}
	})

	t.Run("fails_when_target_node_does_not_exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "Default", EnableBlocking: true},
			},
		}
		helperWriteConfig(t, configPath, cfg)

		// Configure only a single named node via secrets.json
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		if err := os.WriteFile(secretsPath, []byte(`{"nodes":[{"name":"dns1","url":"http://localhost:5380","token":"token1"}]}`), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "deploy", "--node", "nonexistent", "-f"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error when targeting non-existent node, got nil")
		}

		if !strings.Contains(err.Error(), "target node \"nonexistent\" is not defined") {
			t.Errorf("expected error about undefined node, got: %v", err)
		}
	})
}

func TestListNodesCommand(t *testing.T) {
	t.Run("displays_all_configured_nodes", func(t *testing.T) {
		// Two or more nodes require secrets.json; use the "name" field for identity.
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[` +
			`{"name":"dns1","url":"http://dns1.example.com:5380","token":"token1"},` +
			`{"name":"dns2","url":"http://dns2.example.com:5380","token":"token2"},` +
			`{"name":"prod","url":"http://prod.example.com:5380","token":"prod-token"}` +
			`]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"list-nodes"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing list-nodes: %v", err)
		}

		output := outBuf.String()
		// Verify all 3 node names appear in output
		if !strings.Contains(output, "dns1") {
			t.Errorf("expected dns1 node in output: %s", output)
		}
		if !strings.Contains(output, "dns2") {
			t.Errorf("expected dns2 node in output: %s", output)
		}
		if !strings.Contains(output, "prod") {
			t.Errorf("expected prod node in output: %s", output)
		}

		// Verify nodes appear in sorted order
		pos1 := strings.Index(output, "dns1")
		pos2 := strings.Index(output, "dns2")
		pos3 := strings.Index(output, "prod")
		if pos1 < 0 || pos2 < 0 || pos3 < 0 {
			t.Fatalf("could not find all node names in output")
		}
		if !(pos1 < pos2 && pos2 < pos3) {
			t.Errorf("nodes not in sorted order: dns1@%d, dns2@%d, prod@%d", pos1, pos2, pos3)
		}
	})

	t.Run("ignores_legacy_multi-node_env_var_patterns", func(t *testing.T) {
		// The removed TECHNITIUM_NODE_<name>_URL/_TOKEN and TECHNITIUM_URL_<suffix>/
		// TECHNITIUM_TOKEN_<suffix> patterns must no longer register any node.
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		t.Setenv("TECHNITIUM_NODE_DNS1_URL", "http://dns1.example.com:5380")
		t.Setenv("TECHNITIUM_NODE_DNS1_TOKEN", "token1")
		t.Setenv("TECHNITIUM_URL_PROD", "http://prod.example.com:5380")
		t.Setenv("TECHNITIUM_TOKEN_PROD", "prod-token")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"list-nodes"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing list-nodes: %v", err)
		}

		output := outBuf.String()
		if !strings.Contains(output, "No Technitium nodes configured") {
			t.Errorf("expected legacy multi-node env var patterns to register no nodes, got: %s", output)
		}
	})

	t.Run("reports_visible_merge_of_environment_and_secrets_json_sources", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		if err := os.WriteFile(secretsPath, []byte(`{"nodes":[{"name":"backup","url":"http://backup.example.com:5380","token":"tok"}]}`), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"list-nodes"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing list-nodes: %v", err)
		}

		output := outBuf.String()
		if !strings.Contains(output, "Found 1 node(s) from environment variables + 1 from secrets.json = 2 total") {
			t.Errorf("expected visible source merge summary, got: %s", output)
		}
		if !strings.Contains(output, "default") || !strings.Contains(output, "backup") {
			t.Errorf("expected both 'default' and 'backup' nodes in output, got: %s", output)
		}
	})

	t.Run("handles_no_nodes_configured", func(t *testing.T) {
		// Clear any environment variables that might configure nodes
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "TECHNITIUM_") {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 {
					t.Setenv(parts[0], "")
				}
			}
		}

		// Set XDG_CONFIG_HOME to prevent reading real secrets
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"list-nodes"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing list-nodes with no nodes: %v", err)
		}

		output := outBuf.String()
		if !strings.Contains(output, "No Technitium nodes configured") {
			t.Errorf("expected helpful error message about no nodes, got: %s", output)
		}
	})

	t.Run("formats_output_as_table", func(t *testing.T) {
		// Configure 2 named nodes via secrets.json with specific URLs
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[` +
			`{"name":"dns1","url":"http://dns1.example.com:5380","token":"token1"},` +
			`{"name":"dns2","url":"http://dns2.example.com:5380","token":"token2"}` +
			`]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"list-nodes"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error executing list-nodes: %v", err)
		}

		output := outBuf.String()
		// Verify table headers are present
		if !strings.Contains(output, "Node Name") {
			t.Errorf("expected 'Node Name' header, got output: %s", output)
		}
		if !strings.Contains(output, "URL") {
			t.Errorf("expected 'URL' header, got output: %s", output)
		}

		// Verify node names and URLs appear
		if !strings.Contains(output, "dns1") {
			t.Errorf("expected dns1 node name in table, got: %s", output)
		}
		if !strings.Contains(output, "http://dns1.example.com:5380") {
			t.Errorf("expected dns1 URL in table, got: %s", output)
		}
		if !strings.Contains(output, "dns2") {
			t.Errorf("expected dns2 node name in table, got: %s", output)
		}
		if !strings.Contains(output, "http://dns2.example.com:5380") {
			t.Errorf("expected dns2 URL in table, got: %s", output)
		}
	})
}

// TestResolveNodes exercises resolveNodes/resolveNodesWithSources directly against
// the new single-node-env-var + secrets.json(name field) design.
func TestResolveNodes(t *testing.T) {
	t.Run("single TECHNITIUM_URL/TOKEN pair resolves to the default node", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://solo.example.com:5380")
		t.Setenv("TECHNITIUM_TOKEN", "solo-token")

		nodes, counts, err := resolveNodesWithSources()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes) != 1 {
			t.Fatalf("expected exactly 1 node, got %d: %+v", len(nodes), nodes)
		}
		node, ok := nodes["default"]
		if !ok {
			t.Fatalf("expected node named \"default\", got: %+v", nodes)
		}
		if node.URL != "http://solo.example.com:5380" || node.Token != "solo-token" {
			t.Errorf("unexpected node contents: %+v", node)
		}
		if counts.Environment != 1 || counts.SecretsFile != 0 {
			t.Errorf("unexpected source counts: %+v", counts)
		}
	})

	t.Run("legacy TECHNITIUM_NODE_* and TECHNITIUM_URL_<suffix> patterns are ignored", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_NODE_DNS1_URL", "http://dns1.example.com:5380")
		t.Setenv("TECHNITIUM_NODE_DNS1_TOKEN", "token1")
		t.Setenv("TECHNITIUM_URL_PROD", "http://prod.example.com:5380")
		t.Setenv("TECHNITIUM_TOKEN_PROD", "prod-token")

		nodes, counts, err := resolveNodesWithSources()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes) != 0 {
			t.Errorf("expected legacy env var patterns to register no nodes, got: %+v", nodes)
		}
		if counts.Environment != 0 || counts.SecretsFile != 0 {
			t.Errorf("unexpected source counts: %+v", counts)
		}
	})

	t.Run("secrets.json name field is used when present", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		if err := os.WriteFile(secretsPath, []byte(`{"nodes":[{"name":"prod","url":"http://prod.example.com:5380","token":"tok"}]}`), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		nodes, counts, err := resolveNodesWithSources()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := nodes["prod"]; !ok {
			t.Fatalf("expected node named \"prod\", got: %+v", nodes)
		}
		if _, ok := nodes["node-0"]; ok {
			t.Errorf("expected \"name\" field to override node-%%d fallback, got: %+v", nodes)
		}
		if counts.SecretsFile != 1 {
			t.Errorf("expected SecretsFile count of 1, got: %+v", counts)
		}
	})

	t.Run("secrets.json falls back to node-%d naming when name is omitted", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[` +
			`{"url":"http://a.example.com:5380","token":"tok-a"},` +
			`{"name":"b","url":"http://b.example.com:5380","token":"tok-b"},` +
			`{"url":"http://c.example.com:5380","token":"tok-c"}` +
			`]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		nodes, _, err := resolveNodesWithSources()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := nodes["node-0"]; !ok {
			t.Errorf("expected fallback name node-0 for unnamed first entry, got: %+v", nodes)
		}
		if _, ok := nodes["b"]; !ok {
			t.Errorf("expected named node \"b\", got: %+v", nodes)
		}
		if _, ok := nodes["node-2"]; !ok {
			t.Errorf("expected fallback name node-2 for unnamed third entry, got: %+v", nodes)
		}
	})

	t.Run("environment and secrets.json merge with visible source counts", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://solo.example.com:5380")
		t.Setenv("TECHNITIUM_TOKEN", "solo-token")

		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[` +
			`{"name":"backup1","url":"http://backup1.example.com:5380","token":"tok1"},` +
			`{"name":"backup2","url":"http://backup2.example.com:5380","token":"tok2"}` +
			`]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		nodes, counts, err := resolveNodesWithSources()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes) != 3 {
			t.Fatalf("expected 3 total nodes, got %d: %+v", len(nodes), nodes)
		}
		if counts.Environment != 1 {
			t.Errorf("expected Environment count of 1, got %d", counts.Environment)
		}
		if counts.SecretsFile != 2 {
			t.Errorf("expected SecretsFile count of 2, got %d", counts.SecretsFile)
		}
	})

	t.Run("malformed secrets.json surfaces an error instead of looking like zero nodes", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		if err := os.WriteFile(secretsPath, []byte(`{not valid json`), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		nodes, _, err := resolveNodesWithSources()
		if err == nil {
			t.Fatalf("expected an error for malformed secrets.json, got nodes=%+v, err=nil", nodes)
		}
		if !strings.Contains(err.Error(), "parse") {
			t.Errorf("expected parse-error message, got: %v", err)
		}
	})
}
