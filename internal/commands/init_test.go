package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitCommand(t *testing.T) {
	t.Run("--json errors clearly when no credentials are resolvable", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"init", "--json"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error when no credentials are resolvable in --json mode, got nil")
		}
		if !strings.Contains(err.Error(), "no Technitium credentials resolvable") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("--json prints full config JSON and does not write the file", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"blockingAnswerTtl\":30,\"groups\":[{\"name\":\"Default\"}],\"networkGroupMap\":{\"192.168.1.5\":\"Default\"}}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "--json"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		var cfg struct {
			EnableBlocking    bool `json:"enableBlocking"`
			BlockingAnswerTtl int  `json:"blockingAnswerTtl"`
			Groups            []struct {
				Name string `json:"name"`
			} `json:"groups"`
		}
		if err := json.Unmarshal(outBuf.Bytes(), &cfg); err != nil {
			t.Fatalf("expected valid JSON output, got error: %v\noutput: %s", err, outBuf.String())
		}
		if !cfg.EnableBlocking || cfg.BlockingAnswerTtl != 30 || len(cfg.Groups) != 1 || cfg.Groups[0].Name != "Default" {
			t.Errorf("unexpected parsed config: %+v", cfg)
		}

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Errorf("expected --json mode to not write %q, but it exists (err=%v)", configPath, err)
		}
	})

	t.Run("empty server config falls back to template and reports it explicitly", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				// Technitium returns a null config for a fresh instance.
				w.Write([]byte(`{"status": "ok", "response": {"config": null}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "--json"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		var cfg struct {
			Groups []struct {
				Name string `json:"name"`
			} `json:"groups"`
		}
		if err := json.Unmarshal(outBuf.Bytes(), &cfg); err != nil {
			t.Fatalf("expected valid JSON template output, got error: %v\noutput: %s", err, outBuf.String())
		}
		if len(cfg.Groups) != 1 || cfg.Groups[0].Name != "Default" {
			t.Errorf("expected template fallback with a single Default group, got: %+v", cfg)
		}
	})

	t.Run("interactive single-node credential entry prints export lines and previews the fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		configPath := filepath.Join(tmpDir, "dnsApp.config")

		// Prompts in order: URL, token, "manage more than one?" (n), then final
		// save confirmation (n, so we don't need to assert file contents here).
		input := server.URL + "\n" + "mock-token\n" + "n\n" + "n\n"

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(strings.NewReader(input), &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		output := outBuf.String()
		if !strings.Contains(output, "export TECHNITIUM_URL=") || !strings.Contains(output, "export TECHNITIUM_TOKEN=") {
			t.Errorf("expected shell export guidance for single-node setup, got: %s", output)
		}
		if !strings.Contains(output, "Preview (NOT YET SAVED)") {
			t.Errorf("expected a preview section, got: %s", output)
		}
		if !strings.Contains(output, "Not saved.") {
			t.Errorf("expected save to be declined, got: %s", output)
		}
		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Errorf("expected declined save to leave no file, err=%v", err)
		}
	})

	t.Run("interactive multi-node credential entry writes secrets.json with name field at chmod 600", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		configPath := filepath.Join(tmpDir, "dnsApp.config")

		// URL, token, "manage more than one?" (y), name "dns1",
		// "add another?" (n), then final save confirmation (n).
		input := server.URL + "\n" + "mock-token\n" + "y\n" + "dns1\n" + "n\n" + "n\n"

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(strings.NewReader(input), &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		secretsPath := filepath.Join(tmpDir, "pab", "secrets.json")
		fi, statErr := os.Stat(secretsPath)
		if statErr != nil {
			t.Fatalf("expected secrets.json to be written at %q: %v", secretsPath, statErr)
		}
		if runtime.GOOS != "windows" {
			if mode := fi.Mode().Perm(); mode != 0600 {
				t.Errorf("expected secrets.json permissions to be 0600, got %o", mode)
			}
		}

		raw, err := os.ReadFile(secretsPath)
		if err != nil {
			t.Fatalf("failed to read secrets.json: %v", err)
		}
		var sc SecretsConfig
		if err := json.Unmarshal(raw, &sc); err != nil {
			t.Fatalf("failed to parse secrets.json: %v", err)
		}
		if len(sc.Nodes) != 1 || sc.Nodes[0].Name != "dns1" {
			t.Errorf("expected a single node named 'dns1' in secrets.json, got: %+v", sc.Nodes)
		}
	})

	t.Run("re-run against an existing local config diffs before requiring confirmation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"},{\"name\":\"NewGroup\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		existing := []byte(`{"enableBlocking":true,"groups":[{"name":"Default"}]}`)
		if err := os.WriteFile(configPath, existing, 0644); err != nil {
			t.Fatalf("failed to seed existing config: %v", err)
		}

		// Decline the overwrite confirmation.
		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(strings.NewReader("n\n"), &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		output := outBuf.String()
		if !strings.Contains(output, "already exists and differs") {
			t.Errorf("expected a diff-before-overwrite notice, got: %s", output)
		}
		if !strings.Contains(output, `+ Group "NewGroup" (Added)`) {
			t.Errorf("expected the structural diff to list the added group, got: %s", output)
		}
		if !strings.Contains(output, "Not saved.") {
			t.Errorf("expected the decline to be honored, got: %s", output)
		}

		// Existing file must be untouched (no silent clobber).
		onDisk, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read config after declined overwrite: %v", err)
		}
		if string(onDisk) != string(existing) {
			t.Errorf("expected existing config to be untouched, got: %s", string(onDisk))
		}
	})

	t.Run("-f writes without a confirmation prompt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", server.URL)
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		if !strings.Contains(outBuf.String(), "Saved configuration to") {
			t.Errorf("expected a saved confirmation message, got: %s", outBuf.String())
		}
		if _, err := os.Stat(configPath); err != nil {
			t.Fatalf("expected config file to be written with -f: %v", err)
		}
	})

	t.Run("fleet consistency check proceeds silently when nodes agree", func(t *testing.T) {
		configJSON := `{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(configJSON))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[{"name":"prod","url":"` + server.URL + `","token":"tok"},{"name":"backup","url":"` + server.URL + `","token":"tok"}]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		output := outBuf.String()
		if !strings.Contains(output, "All 2 configured nodes agree") {
			t.Errorf("expected agreement message, got: %s", output)
		}
		if strings.Contains(output, "WARNING") {
			t.Errorf("did not expect a disagreement warning, got: %s", output)
		}
	})

	t.Run("fleet consistency check stops and requires --node when nodes disagree", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				callCount++
				// Alternate between two different configs based on call order is
				// unreliable across nodes; instead, key off the "node" identity via
				// a query the test doesn't control, so use a per-server difference:
				// prod and backup are two different httptest servers below.
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"},{\"name\":\"DriftedGroup\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer serverB.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[{"name":"prod","url":"` + server.URL + `","token":"tok"},{"name":"backup","url":"` + serverB.URL + `","token":"tok"}]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "-f"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error when configured nodes disagree, got nil")
		}
		if !strings.Contains(err.Error(), "disagree") || !strings.Contains(err.Error(), "--node") {
			t.Errorf("expected an error demanding explicit --node selection, got: %v", err)
		}

		output := outBuf.String()
		if !strings.Contains(output, "WARNING") {
			t.Errorf("expected a disagreement warning in output, got: %s", output)
		}
		if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
			t.Errorf("expected no file to be written when nodes disagree without --node, err=%v", statErr)
		}
		_ = callCount
	})

	t.Run("fleet consistency check proceeds with --node as the explicit source of truth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"},{\"name\":\"DriftedGroup\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer serverB.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[{"name":"prod","url":"` + server.URL + `","token":"tok"},{"name":"backup","url":"` + serverB.URL + `","token":"tok"}]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "--node", "prod", "-f"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		if _, err := os.Stat(configPath); err != nil {
			t.Fatalf("expected config to be written when --node explicitly selects a source: %v", err)
		}

		onDisk, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read written config: %v", err)
		}
		var cfg struct {
			Groups []struct {
				Name string `json:"name"`
			} `json:"groups"`
		}
		if err := json.Unmarshal(onDisk, &cfg); err != nil {
			t.Fatalf("failed to parse written config: %v", err)
		}
		if len(cfg.Groups) != 1 || cfg.Groups[0].Name != "Default" {
			t.Errorf("expected written config to match the explicitly selected 'prod' node, got: %+v", cfg.Groups)
		}
	})

	t.Run("--json keeps stdout as clean JSON even when nodes disagree", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"},{\"name\":\"DriftedGroup\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer serverB.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[{"name":"prod","url":"` + server.URL + `","token":"tok"},{"name":"backup","url":"` + serverB.URL + `","token":"tok"}]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init", "--node", "prod", "--json"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		// stdout must be nothing but the printed configuration JSON.
		var cfg struct {
			Groups []struct {
				Name string `json:"name"`
			} `json:"groups"`
		}
		if err := json.Unmarshal(outBuf.Bytes(), &cfg); err != nil {
			t.Fatalf("expected stdout to be clean JSON, got parse error %v for: %s", err, outBuf.String())
		}
		if len(cfg.Groups) != 1 || cfg.Groups[0].Name != "Default" {
			t.Errorf("expected the printed config to match the explicitly selected 'prod' node, got: %+v", cfg.Groups)
		}

		// The disagreement warning belongs on stderr, not stdout.
		if !strings.Contains(errBuf.String(), "disagree") {
			t.Errorf("expected the disagreement warning on stderr, got: %s", errBuf.String())
		}
		if strings.Contains(outBuf.String(), "disagree") || strings.Contains(outBuf.String(), "Proceeding with node") {
			t.Errorf("expected no diagnostic text on stdout in --json mode, got: %s", outBuf.String())
		}
	})

	t.Run("fetch/auth error offers a bounded loop-back to credential re-entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://127.0.0.1:1")
		t.Setenv("TECHNITIUM_TOKEN", "bad-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		// Decline the retry prompt so the test terminates deterministically with
		// the original fetch error.
		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(strings.NewReader("n\n"), &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected a fetch error against an unreachable node, got nil")
		}
		if !strings.Contains(errBuf.String(), "Error:") {
			t.Errorf("expected the fetch error to be surfaced plainly, got stderr: %s", errBuf.String())
		}
		if !strings.Contains(outBuf.String(), "Re-enter credentials and try again?") {
			t.Errorf("expected a loop-back prompt offering credential re-entry, got: %s", outBuf.String())
		}
	})

	t.Run("retry loop-back requires confirmation before overwriting an existing secrets.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		original := []byte(`{"nodes":[{"name":"existing1","url":"http://127.0.0.1:1","token":"bad-token"}]}`)
		if err := os.WriteFile(secretsPath, original, 0600); err != nil {
			t.Fatalf("failed to seed secrets.json: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		// The seeded "existing1" node is unreachable, so the initial fetch fails.
		// Accept the retry prompt, enter fresh single-node-turned-multi-node
		// credentials, then DECLINE the resulting "overwrite secrets.json?"
		// confirmation -- the existing file must survive untouched.
		input := "y\n" + // retry?
			"http://127.0.0.1:2\n" + // new URL (also unreachable, never reached)
			"new-token\n" + // new token
			"y\n" + // manage more than one?
			"newnode\n" + // name
			"n\n" + // add another?
			"n\n" // decline the overwrite confirmation

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(strings.NewReader(input), &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error since the overwrite confirmation was declined")
		}
		if !strings.Contains(err.Error(), "not overwriting existing") {
			t.Errorf("expected an overwrite-declined error, got: %v", err)
		}

		onDisk, readErr := os.ReadFile(secretsPath)
		if readErr != nil {
			t.Fatalf("failed to read secrets.json after declined overwrite: %v", readErr)
		}
		if string(onDisk) != string(original) {
			t.Errorf("expected secrets.json to be untouched by a declined overwrite, got: %s", string(onDisk))
		}
	})

	t.Run("retry loop-back overwrites secrets.json once the user confirms", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/apps/config/get" {
				w.Write([]byte(`{"status": "ok", "response": {"config": "{\"enableBlocking\":true,\"groups\":[{\"name\":\"Default\"}]}"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		original := []byte(`{"nodes":[{"name":"existing1","url":"http://127.0.0.1:1","token":"bad-token"}]}`)
		if err := os.WriteFile(secretsPath, original, 0600); err != nil {
			t.Fatalf("failed to seed secrets.json: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")

		input := "y\n" + // retry?
			server.URL + "\n" + // new URL (reachable this time)
			"new-token\n" + // new token
			"y\n" + // manage more than one?
			"newnode\n" + // name
			"n\n" + // add another?
			"y\n" + // confirm the overwrite
			"n\n" // decline the final "save dnsApp.config?" prompt

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(strings.NewReader(input), &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "init"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		raw, err := os.ReadFile(secretsPath)
		if err != nil {
			t.Fatalf("failed to read secrets.json: %v", err)
		}
		var sc SecretsConfig
		if err := json.Unmarshal(raw, &sc); err != nil {
			t.Fatalf("failed to parse secrets.json: %v", err)
		}
		if len(sc.Nodes) != 1 || sc.Nodes[0].Name != "newnode" {
			t.Errorf("expected secrets.json to be overwritten with a single 'newnode' entry, got: %+v", sc.Nodes)
		}
		if strings.Contains(string(raw), "existing1") {
			t.Errorf("expected the confirmed overwrite to replace the original node, got: %s", string(raw))
		}
	})
}
