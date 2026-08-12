package lxdbackend

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/canonical/workshop/internal/logger"
)

const firewallDocLink = "https://canonical.com/lxd/docs/latest/howto/network_bridge_firewalld/"

// CheckBridgeFirewall detects whether firewall rules block forwarding on the
// workshop bridge. It returns a non-empty warning message with a proposed
// resolution if an issue is detected, or an empty string if everything looks
// fine.
//
// Detection is cause-agnostic: it checks whether the FORWARD chain policy is
// DROP/REJECT and whether any rules ACCEPT traffic for the bridge. The
// remediation advice is cause-aware: it suggests Docker-specific, UFW-specific,
// or generic commands depending on what is found.
//
// Uses `nft -j` (JSON output) for structured, robust parsing.
//
// See: https://canonical.com/lxd/docs/latest/howto/network_bridge_firewalld/
func CheckBridgeFirewall(bridgeName string) string {
	return firewallChecker(bridgeName)
}

var firewallChecker = checkBridgeFirewall

func checkBridgeFirewall(bridgeName string) string {
	nft, err := exec.LookPath("nft")
	if err != nil {
		return ""
	}

	out, err := exec.Command(nft, "-j", "list", "table", "ip", "filter").CombinedOutput()
	if err != nil {
		logger.Noticef("%s", out)
		return "" // no filter table
	}

	return analyzeNftJSON(out, bridgeName)
}

// nftRuleset represents the top-level nft JSON output.
type nftRuleset struct {
	Nftables []nftObject `json:"nftables"`
}

// nftObject is a single entry in the nftables array. Each entry contains
// exactly one of the possible object types.
type nftObject struct {
	Chain *nftChain `json:"chain,omitempty"`
	Rule  *nftRule  `json:"rule,omitempty"`
}

// nftChain represents a chain object from nft JSON output.
type nftChain struct {
	Family string `json:"family"`
	Table  string `json:"table"`
	Name   string `json:"name"`
	Policy string `json:"policy"`
}

// nftRule represents a rule object from nft JSON output. We only need the
// raw expression array to scan for interface references and accept verdicts.
type nftRule struct {
	Family string            `json:"family"`
	Table  string            `json:"table"`
	Chain  string            `json:"chain"`
	Expr   []json.RawMessage `json:"expr"`
}

// analyzeNftJSON parses nft JSON output and returns a warning if forwarding
// is blocked for the bridge.
func analyzeNftJSON(data []byte, bridgeName string) string {
	var ruleset nftRuleset
	if err := json.Unmarshal(data, &ruleset); err != nil {
		return ""
	}

	if !forwardPolicyIsDrop(ruleset) {
		return ""
	}

	if bridgeHasAcceptRule(ruleset, bridgeName) {
		return ""
	}

	return bridgeBlockedWarning(bridgeName, detectFirewallCause(ruleset))
}

// forwardPolicyIsDrop returns true if the FORWARD chain has a "drop" policy.
func forwardPolicyIsDrop(ruleset nftRuleset) bool {
	for _, obj := range ruleset.Nftables {
		if obj.Chain != nil && obj.Chain.Name == "FORWARD" {
			return obj.Chain.Policy == "drop"
		}
	}
	return false
}

// bridgeHasAcceptRule checks whether any rule in the filter table references
// the bridge interface and has an accept verdict.
func bridgeHasAcceptRule(ruleset nftRuleset, bridgeName string) bool {
	for _, obj := range ruleset.Nftables {
		if obj.Rule == nil {
			continue
		}
		if ruleMatchesBridge(obj.Rule.Expr, bridgeName) && ruleHasAccept(obj.Rule.Expr) {
			return true
		}
	}
	return false
}

// ruleMatchesBridge returns true if any expression in the rule references the
// bridge interface name. In nft JSON, interface matches look like:
//
//	{"match": {"left": {"meta": {"key": "iifname"}}, "right": "workshopbr0", ...}}
//
// We check for the bridge name appearing as a string value anywhere in the
// expression tree.
func ruleMatchesBridge(exprs []json.RawMessage, bridgeName string) bool {
	for _, expr := range exprs {
		if strings.Contains(string(expr), bridgeName) {
			return true
		}
	}
	return false
}

// ruleHasAccept returns true if any expression in the rule is an accept
// verdict: {"accept": null}.
func ruleHasAccept(exprs []json.RawMessage) bool {
	for _, expr := range exprs {
		if strings.Contains(string(expr), `"accept"`) {
			return true
		}
	}
	return false
}

// firewallCause identifies the likely source of the FORWARD DROP policy to
// tailor the remediation advice.
type firewallCause int

const (
	causeUnknown firewallCause = iota
	causeDocker
	causeUFW
)

// detectFirewallCause inspects chain and rule names for markers of known
// firewall software.
func detectFirewallCause(ruleset nftRuleset) firewallCause {
	for _, obj := range ruleset.Nftables {
		if obj.Chain != nil && strings.Contains(obj.Chain.Name, "DOCKER") {
			return causeDocker
		}
		if obj.Rule != nil && strings.Contains(obj.Rule.Chain, "ufw") {
			return causeUFW
		}
	}
	for _, obj := range ruleset.Nftables {
		if obj.Chain != nil && strings.Contains(obj.Chain.Name, "ufw") {
			return causeUFW
		}
	}
	return causeUnknown
}

func bridgeBlockedWarning(bridgeName string, cause firewallCause) string {
	base := "firewall rules may be blocking network traffic on the " + bridgeName +
		" bridge: the FORWARD chain policy is set to DROP with no rules " +
		"allowing traffic through the bridge"

	switch cause {
	case causeDocker:
		return base + ". " +
			"This is likely caused by Docker. To resolve, run: " +
			"sudo nft insert rule ip filter DOCKER-USER iifname " + bridgeName + " accept \\; " +
			"sudo nft insert rule ip filter DOCKER-USER oifname " + bridgeName +
			" ct state related,established accept" +
			" (see " + firewallDocLink + ")"
	case causeUFW:
		return base + ". " +
			"This is likely caused by UFW. To resolve, run: " +
			"sudo ufw allow in on " + bridgeName + " && " +
			"sudo ufw route allow in on " + bridgeName + " && " +
			"sudo ufw route allow out on " + bridgeName +
			" (see " + firewallDocLink + ")"
	default:
		return base + ". " +
			"To resolve, add firewall rules allowing forwarding through " + bridgeName +
			" (see " + firewallDocLink + ")"
	}
}
