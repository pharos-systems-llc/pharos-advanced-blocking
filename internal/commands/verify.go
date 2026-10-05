package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pharos-systems-llc/pharos-advanced-blocking/internal/config"
	"github.com/spf13/cobra"
)

// dnsQueryTimeout bounds the live DNS query pab verify performs so a
// misconfigured/unreachable node doesn't hang the command indefinitely.
const dnsQueryTimeout = 5 * time.Second

// dnsLookupFunc queries the resolver at addr (a "host:port" pair) for domain's
// A/AAAA records and returns the resolved IP addresses. It mirrors the shape of
// net.Resolver.LookupHost. A non-nil error wrapping a *net.DNSError with
// IsNotFound == true represents an authoritative NXDOMAIN-style response, which
// evaluateVerification treats as a distinct, meaningful outcome rather than a
// generic failure.
//
// This is a package-level variable -- the same injection technique
// client.Client uses via its exported HTTPClient field -- so tests can
// substitute a fake resolver instead of requiring a live DNS server. Unlike
// client.Client, newVerifyCmd doesn't construct a long-lived struct its callers
// hold onto, so a package variable is the simplest seam available; tests must
// restore the default after overriding it.
type dnsLookupFunc func(ctx context.Context, addr, domain string) ([]string, error)

var dnsLookup dnsLookupFunc = defaultDNSLookup

// defaultDNSLookup is the real implementation: it points a Go-native
// *net.Resolver directly at the target node's DNS listener (bypassing the
// system resolver/cache entirely) and performs a live A/AAAA lookup.
func defaultDNSLookup(ctx context.Context, addr, domain string) ([]string, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: dnsQueryTimeout}
			return d.DialContext(ctx, network, addr)
		},
	}
	return resolver.LookupHost(ctx, domain)
}

// VerifyResult is the machine-readable outcome of `pab verify`, printed
// directly as --json output and used to derive the human-readable summary.
type VerifyResult struct {
	Domain            string   `json:"domain"`
	Node              string   `json:"node"`
	QueryAddress      string   `json:"queryAddress"`
	Group             string   `json:"group"`
	BlockAsNxDomain   bool     `json:"blockAsNxDomain"`
	ExpectedAddresses []string `json:"expectedAddresses,omitempty"`
	NXDomain          bool     `json:"nxdomain"`
	ResolvedAddresses []string `json:"resolvedAddresses,omitempty"`
	// Status is one of "blocked", "not-blocked", "inconclusive", or "error".
	// Only "blocked" is treated as success (see newVerifyCmd's exit-code logic).
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// newVerifyCmd defines the "pab verify --domain <domain> [--node <name>]
// [--group <name>] [--port <port>] [--json]" command: it closes the deploy loop
// by actually querying a node's DNS resolver for a domain and reporting whether
// the live response matches the domain's group's configured blocking behavior
// (Group.BlockAsNxDomain / Group.BlockingAddresses), rather than merely
// confirming a config was PUT to the API.
func newVerifyCmd(global *GlobalFlags, stdout, stderr io.Writer) *cobra.Command {
	var domain string
	var targetNode string
	var groupName string
	var port int
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "verify --domain <domain> [--node <name>] [--group <name>] [--port <port>] [--json]",
		Short: "Query a node's live DNS resolver to confirm a domain is actually being blocked",
		// verify legitimately prints a result (including full --json output) and
		// then returns a non-nil error for any non-"blocked" outcome, so
		// automation can rely on the exit code. Without SilenceUsage, cobra's
		// default error handling would append a "Usage: ..." block to the same
		// stdout writer, corrupting --json output; SilenceErrors is left at its
		// default so cobra still surfaces "Error: <message>" on stderr,
		// consistent with every other command in this CLI.
		SilenceUsage: true,
		Long: `Resolves the target Technitium node the same way deploy/init do, then sends a
live DNS query for --domain directly to that node's resolver and compares the
response against the configured blocking behavior of the group the domain
belongs to (Group.BlockAsNxDomain / Group.BlockingAddresses in the locally
loaded configuration).

The group is determined automatically by searching the loaded config's groups
for one whose Blocked or BlockedRegex entries match --domain. Domains only
covered indirectly via a remote BlockListUrls/AdblockListUrls entry cannot be
attributed automatically (pab does not fetch and parse those remote lists) --
pass --group explicitly in that case.

v1 targets DNS port 53 by default (Technitium's default listener port); use
--port to override for a non-default deployment. This command exits non-zero
unless the live response is conclusively confirmed as blocked, so it is safe
to use directly in CI/automation after "pab deploy".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if domain == "" {
				return errors.New("missing required flag: --domain")
			}

			cfg, err := loadConfig(global.ConfigFile)
			if err != nil {
				return err
			}

			group, err := resolveGroupForDomain(cfg, domain, groupName, global.ConfigFile)
			if err != nil {
				return err
			}

			nodes, counts, err := resolveNodesWithSources()
			if err != nil {
				return err
			}
			if len(nodes) == 0 {
				return errors.New("no Technitium nodes configured. Set environment variables or define them in ~/.config/pab/secrets.json")
			}

			nodeName, node, err := selectVerifyNode(nodes, targetNode)
			if err != nil {
				return err
			}

			host, err := dnsHostFromNodeURL(node.URL)
			if err != nil {
				return fmt.Errorf("failed to determine DNS host for node %q: %w", nodeName, err)
			}
			addr := net.JoinHostPort(host, strconv.Itoa(port))

			if !jsonOutput {
				fmt.Fprintf(stdout, "Querying %s (node %q) for %q...\n", addr, nodeName, domain)
				if counts.Environment > 0 && counts.SecretsFile > 0 {
					fmt.Fprintf(stdout, "(resolved %d node(s): %d from environment variables, %d from secrets.json)\n", len(nodes), counts.Environment, counts.SecretsFile)
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), dnsQueryTimeout)
			defer cancel()
			ips, lookupErr := dnsLookup(ctx, addr, domain)

			result := evaluateVerification(domain, nodeName, addr, group, ips, lookupErr)

			if jsonOutput {
				out, marshalErr := json.MarshalIndent(result, "", "  ")
				if marshalErr != nil {
					return fmt.Errorf("failed to format verification result as JSON: %w", marshalErr)
				}
				fmt.Fprintln(stdout, string(out))
			} else {
				printVerifyResult(stdout, result)
			}

			if result.Status == "error" {
				return fmt.Errorf("DNS query failed: %s", result.Detail)
			}
			if result.Status != "blocked" {
				return fmt.Errorf("domain %q does not appear to be blocked on node %q: %s", domain, nodeName, result.Detail)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "Domain to test-query against the node's DNS resolver (required)")
	cmd.Flags().StringVar(&targetNode, "node", "", "Node to query (required when more than one node is configured)")
	cmd.Flags().StringVar(&groupName, "group", "", "Explicit group to verify against, for domains that can't be unambiguously attributed to one")
	cmd.Flags().IntVar(&port, "port", 53, "DNS port to query on the target node (Technitium's default DNS listener port)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print the verification result as JSON")

	return cmd
}

// resolveGroupForDomain attributes domain to one of cfg's groups so its
// expected blocking behavior (BlockAsNxDomain/BlockingAddresses) can be
// checked. pab verify takes a domain, not a group, so this searches each
// group's literal Blocked and BlockedRegex entries for a match; it does not
// fetch or parse remote BlockListUrls/AdblockListUrls contents, since doing so
// would require downloading and parsing arbitrary external block lists just to
// answer "which group is this in" -- out of scope for a v1 verification
// command. explicitGroup, when set, bypasses the search entirely (required
// when a domain is only covered by such a remote list).
func resolveGroupForDomain(cfg *config.Config, domain, explicitGroup, configPath string) (*config.Group, error) {
	if explicitGroup != "" {
		for i := range cfg.Groups {
			if cfg.Groups[i].Name == explicitGroup {
				return &cfg.Groups[i], nil
			}
		}
		return nil, fmt.Errorf("group %q not found in %s", explicitGroup, configPath)
	}

	normalized := strings.ToLower(strings.TrimSuffix(domain, "."))

	var matchNames []string
	var matchGroups []*config.Group
	for i := range cfg.Groups {
		g := &cfg.Groups[i]

		matched := slices.ContainsFunc(g.Blocked, func(d string) bool {
			return strings.ToLower(strings.TrimSuffix(d, ".")) == normalized
		})

		if !matched {
			for _, pattern := range g.BlockedRegex {
				re, compileErr := regexp.Compile(pattern)
				if compileErr != nil {
					continue
				}
				if re.MatchString(domain) {
					matched = true
					break
				}
			}
		}

		if matched {
			matchNames = append(matchNames, g.Name)
			matchGroups = append(matchGroups, g)
		}
	}

	switch len(matchGroups) {
	case 0:
		return nil, fmt.Errorf("domain %q was not found in any group's blocked-domain or blocked-regex list in %s; pass --group to specify which group's blocking behavior to verify against (remote blockListUrls/adblockListUrls contents are not fetched or searched)", domain, configPath)
	case 1:
		return matchGroups[0], nil
	default:
		return nil, fmt.Errorf("domain %q matches more than one group (%s); pass --group to disambiguate", domain, strings.Join(matchNames, ", "))
	}
}

// selectVerifyNode picks the single node to query: the explicit --node
// selection when given, the sole resolved node when there's exactly one, or an
// error demanding --node when the target would otherwise be ambiguous. Unlike
// deploy (which fans out to every resolved node), verify performs one live DNS
// query, so silently picking an arbitrary node when several are configured
// would be misleading.
func selectVerifyNode(nodes map[string]NodeConfig, targetNode string) (string, NodeConfig, error) {
	if targetNode != "" {
		node, ok := nodes[targetNode]
		if !ok {
			return "", NodeConfig{}, fmt.Errorf("target node %q is not defined in the resolved credentials", targetNode)
		}
		return targetNode, node, nil
	}

	if len(nodes) == 1 {
		for name, node := range nodes {
			return name, node, nil
		}
	}

	var names []string
	for name := range nodes {
		names = append(names, name)
	}
	slices.Sort(names)
	return "", NodeConfig{}, fmt.Errorf("more than one node is configured (%s); pass --node to select which one to query", strings.Join(names, ", "))
}

// dnsHostFromNodeURL extracts just the hostname/IP from a node's configured API
// URL (e.g. "https://dns.example.com:5380" -> "dns.example.com"), since the API
// port has no bearing on the DNS listener's port.
func dnsHostFromNodeURL(rawURL string) (string, error) {
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		return u.Hostname(), nil
	}

	// Fall back to treating rawURL as a bare host or host:port pair.
	if h, _, err := net.SplitHostPort(rawURL); err == nil && h != "" {
		return h, nil
	}
	if rawURL != "" {
		return rawURL, nil
	}

	return "", fmt.Errorf("could not determine a DNS host from node URL %q", rawURL)
}

// evaluateVerification compares a live DNS lookup's outcome against a group's
// configured blocking behavior and classifies the result. Only Status ==
// "blocked" is treated as a confirmed pass by newVerifyCmd.
func evaluateVerification(domain, nodeName, addr string, group *config.Group, ips []string, lookupErr error) VerifyResult {
	result := VerifyResult{
		Domain:            domain,
		Node:              nodeName,
		QueryAddress:      addr,
		Group:             group.Name,
		BlockAsNxDomain:   group.BlockAsNxDomain,
		ExpectedAddresses: group.BlockingAddresses,
	}

	isNXDomain := false
	if lookupErr != nil {
		var dnsErr *net.DNSError
		if errors.As(lookupErr, &dnsErr) && dnsErr.IsNotFound {
			isNXDomain = true
		} else {
			result.Status = "error"
			result.Detail = lookupErr.Error()
			return result
		}
	}

	result.NXDomain = isNXDomain
	result.ResolvedAddresses = ips

	if group.BlockAsNxDomain {
		if isNXDomain {
			result.Status = "blocked"
			result.Detail = "node returned NXDOMAIN, matching the group's blockAsNxDomain setting"
		} else {
			result.Status = "not-blocked"
			result.Detail = fmt.Sprintf("expected NXDOMAIN (group %q has blockAsNxDomain=true) but the node resolved: %s", group.Name, strings.Join(ips, ", "))
		}
		return result
	}

	// Sinkhole/blocking-address style group.
	if isNXDomain {
		result.Status = "not-blocked"
		result.Detail = fmt.Sprintf("expected a sinkhole address response (group %q has blockAsNxDomain=false) but the node returned NXDOMAIN", group.Name)
		return result
	}

	if len(group.BlockingAddresses) == 0 {
		result.Status = "inconclusive"
		result.Detail = fmt.Sprintf("node resolved %s, but group %q has no configured blockingAddresses to compare against, so pab cannot confirm this is a blocking response (known v1 limitation)", strings.Join(ips, ", "), group.Name)
		return result
	}

	matched := false
	for _, ip := range ips {
		if slices.Contains(group.BlockingAddresses, ip) {
			matched = true
			break
		}
	}

	if matched {
		result.Status = "blocked"
		result.Detail = fmt.Sprintf("node resolved %s, matching a configured blockingAddresses entry", strings.Join(ips, ", "))
	} else {
		result.Status = "not-blocked"
		result.Detail = fmt.Sprintf("node resolved %s, which does not match any configured blockingAddresses (%s)", strings.Join(ips, ", "), strings.Join(group.BlockingAddresses, ", "))
	}
	return result
}

// printVerifyResult renders a VerifyResult as human-readable output.
func printVerifyResult(w io.Writer, r VerifyResult) {
	fmt.Fprintln(w)
	switch r.Status {
	case "blocked":
		fmt.Fprintf(w, "BLOCKED: %q is being blocked as expected by group %q on node %q\n", r.Domain, r.Group, r.Node)
	case "not-blocked":
		fmt.Fprintf(w, "NOT BLOCKED: %q does not appear to be blocked by group %q on node %q\n", r.Domain, r.Group, r.Node)
	case "inconclusive":
		fmt.Fprintf(w, "INCONCLUSIVE: could not confirm blocking for %q against group %q on node %q\n", r.Domain, r.Group, r.Node)
	case "error":
		fmt.Fprintf(w, "ERROR: DNS query for %q against node %q failed\n", r.Domain, r.Node)
	}
	fmt.Fprintf(w, "  %s\n", r.Detail)
	fmt.Fprintln(w)
}
