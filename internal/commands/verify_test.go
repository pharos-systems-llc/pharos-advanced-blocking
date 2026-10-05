package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/config"
)

// withDNSLookup temporarily swaps the package-level dnsLookup hook for a fake
// implementation and restores the real one on cleanup, so tests never touch a
// live DNS server.
func withDNSLookup(t *testing.T, fn dnsLookupFunc) {
	t.Helper()
	original := dnsLookup
	dnsLookup = fn
	t.Cleanup(func() { dnsLookup = original })
}

// nxdomainErr builds a *net.DNSError with IsNotFound set, matching what Go's
// resolver returns for an authoritative NXDOMAIN response.
func nxdomainErr(domain string) error {
	return &net.DNSError{
		Err:        "no such host",
		Name:       domain,
		IsNotFound: true,
	}
}

func writeVerifyTestConfig(t *testing.T, path string) {
	t.Helper()
	cfg := &config.Config{
		EnableBlocking: true,
		Groups: []config.Group{
			{
				Name:            "NxDomainGroup",
				EnableBlocking:  true,
				BlockAsNxDomain: true,
				Blocked:         []string{"nx.example.com"},
			},
			{
				Name:              "SinkholeGroup",
				EnableBlocking:    true,
				BlockAsNxDomain:   false,
				BlockingAddresses: []string{"0.0.0.0"},
				Blocked:           []string{"sinkhole.example.com"},
			},
			{
				Name:            "RegexGroup",
				EnableBlocking:  true,
				BlockAsNxDomain: true,
				BlockedRegex:    []string{`^ads\..*\.example\.com$`},
			},
			{
				Name:            "NoAddressesGroup",
				EnableBlocking:  true,
				BlockAsNxDomain: false,
				Blocked:         []string{"noaddr.example.com"},
			},
		},
	}
	bytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(path, bytes, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}

func TestVerifyCommand(t *testing.T) {
	t.Run("success path: NXDOMAIN-style group blocked as expected", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			if domain != "nx.example.com" {
				t.Errorf("unexpected lookup domain: %s", domain)
			}
			return nil, nxdomainErr(domain)
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		output := outBuf.String()
		if !strings.Contains(output, "BLOCKED:") {
			t.Errorf("expected a BLOCKED result, got: %s", output)
		}
		if !strings.Contains(output, "NxDomainGroup") {
			t.Errorf("expected the attributed group name in output, got: %s", output)
		}
	})

	t.Run("success path: blocking-address-style group blocked as expected", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			if domain != "sinkhole.example.com" {
				t.Errorf("unexpected lookup domain: %s", domain)
			}
			return []string{"0.0.0.0"}, nil
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "sinkhole.example.com"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		output := outBuf.String()
		if !strings.Contains(output, "BLOCKED:") {
			t.Errorf("expected a BLOCKED result, got: %s", output)
		}
		if !strings.Contains(output, "SinkholeGroup") {
			t.Errorf("expected the attributed group name in output, got: %s", output)
		}
	})

	t.Run("regex-matched domain is attributed to the right group", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return nil, nxdomainErr(domain)
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "ads.tracker.example.com"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		if !strings.Contains(outBuf.String(), "RegexGroup") {
			t.Errorf("expected regex-based attribution to RegexGroup, got: %s", outBuf.String())
		}
	})

	t.Run("mismatch: node resolves a live answer for an NXDOMAIN-expected group", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return []string{"93.184.216.34"}, nil
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error when the domain is not actually blocked, got nil")
		}
		if !strings.Contains(err.Error(), "does not appear to be blocked") {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(outBuf.String(), "NOT BLOCKED:") {
			t.Errorf("expected a NOT BLOCKED result in output, got: %s", outBuf.String())
		}
	})

	t.Run("inconclusive when a sinkhole-style group has no configured blockingAddresses", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return []string{"10.0.0.1"}, nil
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "noaddr.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected a non-nil error for an inconclusive result (only 'blocked' is success), got nil")
		}
		if !strings.Contains(outBuf.String(), "INCONCLUSIVE:") {
			t.Errorf("expected an INCONCLUSIVE result in output, got: %s", outBuf.String())
		}
	})

	t.Run("domain not found in any group returns a clear error", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "unknown.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error for a domain not attributable to any group, got nil")
		}
		if !strings.Contains(err.Error(), "was not found in any group") {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(err.Error(), "--group") {
			t.Errorf("expected the error to mention --group as the escape hatch, got: %v", err)
		}
	})

	t.Run("domain matching multiple groups is ambiguous without --group", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "GroupA", BlockAsNxDomain: true, Blocked: []string{"dup.example.com"}},
				{Name: "GroupB", BlockAsNxDomain: false, BlockingAddresses: []string{"0.0.0.0"}, Blocked: []string{"dup.example.com"}},
			},
		}
		b, _ := json.Marshal(cfg)
		if err := os.WriteFile(configPath, b, 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "dup.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an ambiguity error, got nil")
		}
		if !strings.Contains(err.Error(), "matches more than one group") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("--group explicitly disambiguates when set", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return []string{"0.0.0.0"}, nil
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		cfg := &config.Config{
			Groups: []config.Group{
				{Name: "GroupA", BlockAsNxDomain: true, Blocked: []string{"dup.example.com"}},
				{Name: "GroupB", BlockAsNxDomain: false, BlockingAddresses: []string{"0.0.0.0"}, Blocked: []string{"dup.example.com"}},
			},
		}
		b, _ := json.Marshal(cfg)
		if err := os.WriteFile(configPath, b, 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "dup.example.com", "--group", "GroupB"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}
		if !strings.Contains(outBuf.String(), "GroupB") {
			t.Errorf("expected explicit --group selection to be honored, got: %s", outBuf.String())
		}
	})

	t.Run("node resolution failure: no nodes configured", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "")
		t.Setenv("TECHNITIUM_TOKEN", "")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error when no nodes are configured, got nil")
		}
		if !strings.Contains(err.Error(), "no Technitium nodes configured") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("node resolution failure: ambiguous target across multiple nodes", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "")
		t.Setenv("TECHNITIUM_TOKEN", "")
		pabConfigDir := filepath.Join(tmpDir, "pab")
		if err := os.MkdirAll(pabConfigDir, 0755); err != nil {
			t.Fatalf("failed to create mock config dir: %v", err)
		}
		secretsPath := filepath.Join(pabConfigDir, "secrets.json")
		secretsJSON := `{"nodes":[{"name":"prod","url":"http://prod.example.internal:5380","token":"t"},{"name":"backup","url":"http://backup.example.internal:5380","token":"t"}]}`
		if err := os.WriteFile(secretsPath, []byte(secretsJSON), 0600); err != nil {
			t.Fatalf("failed to write secrets file: %v", err)
		}

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error when more than one node is configured without --node, got nil")
		}
		if !strings.Contains(err.Error(), "more than one node is configured") || !strings.Contains(err.Error(), "--node") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("node resolution failure: unknown --node value", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com", "--node", "does-not-exist"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error for an unknown --node value, got nil")
		}
		if !strings.Contains(err.Error(), `target node "does-not-exist" is not defined`) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing --domain flag errors clearly", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error when --domain is omitted, got nil")
		}
		if !strings.Contains(err.Error(), "missing required flag: --domain") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("--json prints the full VerifyResult shape and succeeds for a blocked result", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return nil, nxdomainErr(domain)
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com", "--json"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errBuf.String())
		}

		var result VerifyResult
		if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
			t.Fatalf("expected valid JSON output, got error: %v\noutput: %s", err, outBuf.String())
		}

		if result.Domain != "nx.example.com" {
			t.Errorf("expected domain nx.example.com, got %q", result.Domain)
		}
		if result.Node != "default" {
			t.Errorf("expected node 'default' (single env-var node), got %q", result.Node)
		}
		if result.Group != "NxDomainGroup" {
			t.Errorf("expected group NxDomainGroup, got %q", result.Group)
		}
		if !result.BlockAsNxDomain {
			t.Errorf("expected blockAsNxDomain true, got false")
		}
		if !result.NXDomain {
			t.Errorf("expected nxdomain true, got false")
		}
		if result.Status != "blocked" {
			t.Errorf("expected status 'blocked', got %q", result.Status)
		}
		if result.QueryAddress == "" {
			t.Errorf("expected a non-empty queryAddress")
		}
		if result.Detail == "" {
			t.Errorf("expected a non-empty detail message")
		}

		if strings.Contains(outBuf.String(), "Querying") {
			t.Errorf("expected --json mode to suppress human-readable progress output, got: %s", outBuf.String())
		}
	})

	t.Run("--json still exits non-zero for a not-blocked result", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return []string{"93.184.216.34"}, nil
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com", "--json"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected a non-nil error for a not-blocked result even in --json mode, got nil")
		}

		var result VerifyResult
		if jsonErr := json.Unmarshal(outBuf.Bytes(), &result); jsonErr != nil {
			t.Fatalf("expected valid JSON output despite the failure exit code, got error: %v\noutput: %s", jsonErr, outBuf.String())
		}
		if result.Status != "not-blocked" {
			t.Errorf("expected status 'not-blocked', got %q", result.Status)
		}
	})

	t.Run("a non-NXDOMAIN lookup failure is reported as a query error, not a blocking mismatch", func(t *testing.T) {
		withDNSLookup(t, func(ctx context.Context, addr, domain string) ([]string, error) {
			return nil, errors.New("connection refused")
		})

		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		t.Setenv("TECHNITIUM_URL", "http://technitium.example.internal:5380")
		t.Setenv("TECHNITIUM_TOKEN", "mock-token")

		configPath := filepath.Join(tmpDir, "dnsApp.config")
		writeVerifyTestConfig(t, configPath)

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error for a failed DNS query, got nil")
		}
		if !strings.Contains(err.Error(), "DNS query failed") {
			t.Errorf("expected a distinct 'DNS query failed' error, got: %v", err)
		}
		if !strings.Contains(outBuf.String(), "ERROR:") {
			t.Errorf("expected an ERROR result in output, got: %s", outBuf.String())
		}
	})

	t.Run("missing local config file errors clearly", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)
		configPath := filepath.Join(tmpDir, "does-not-exist.config")

		var outBuf, errBuf bytes.Buffer
		rootCmd := NewRootCmd(nil, &outBuf, &errBuf, "dev", "none", "unknown")
		rootCmd.SetArgs([]string{"--config", configPath, "verify", "--domain", "nx.example.com"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected an error when the local config file is missing, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestDnsHostFromNodeURL(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "http://dns.example.com:5380", want: "dns.example.com"},
		{in: "https://192.168.1.10:5380", want: "192.168.1.10"},
		{in: "dns.example.com:5380", want: "dns.example.com"},
		{in: "dns.example.com", want: "dns.example.com"},
		{in: "", want: "", wantErr: true},
	}

	for _, tc := range cases {
		got, err := dnsHostFromNodeURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("dnsHostFromNodeURL(%q): expected error, got nil", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("dnsHostFromNodeURL(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("dnsHostFromNodeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
