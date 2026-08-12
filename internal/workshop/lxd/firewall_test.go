package lxdbackend

import (
	"gopkg.in/check.v1"
)

type FirewallTests struct{}

var _ = check.Suite(&FirewallTests{})

// nft -j output samples for testing. These mirror real `nft -j list table ip
// filter` output with the metainfo object omitted for brevity.

const nftJSONDockerDrop = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "INPUT", "handle": 1, "type": "filter", "hook": "input", "prio": 0, "policy": "accept"}},
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 2, "type": "filter", "hook": "forward", "prio": 0, "policy": "drop"}},
	{"chain": {"family": "ip", "table": "filter", "name": "OUTPUT", "handle": 3, "type": "filter", "hook": "output", "prio": 0, "policy": "accept"}},
	{"chain": {"family": "ip", "table": "filter", "name": "DOCKER-USER", "handle": 4}},
	{"chain": {"family": "ip", "table": "filter", "name": "DOCKER-ISOLATION-STAGE-1", "handle": 5}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"jump": {"target": "DOCKER-USER"}}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"jump": {"target": "DOCKER-ISOLATION-STAGE-1"}}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"match": {"left": {"meta": {"key": "oifname"}}, "op": "==", "right": "docker0"}}, {"match": {"left": {"ct": {"key": "state"}}, "op": "in", "right": ["established", "related"]}}, {"accept": null}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "DOCKER-USER", "expr": [{"return": null}]}}
]}`

const nftJSONDockerDropWithBridge = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 2, "type": "filter", "hook": "forward", "prio": 0, "policy": "drop"}},
	{"chain": {"family": "ip", "table": "filter", "name": "DOCKER-USER", "handle": 4}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"jump": {"target": "DOCKER-USER"}}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "DOCKER-USER", "expr": [{"match": {"left": {"meta": {"key": "iifname"}}, "op": "==", "right": "workshopbr0"}}, {"accept": null}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "DOCKER-USER", "expr": [{"match": {"left": {"meta": {"key": "oifname"}}, "op": "==", "right": "workshopbr0"}}, {"match": {"left": {"ct": {"key": "state"}}, "op": "in", "right": ["established", "related"]}}, {"accept": null}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "DOCKER-USER", "expr": [{"return": null}]}}
]}`

const nftJSONAcceptPolicy = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 1, "type": "filter", "hook": "forward", "prio": 0, "policy": "accept"}}
]}`

const nftJSONUFWDrop = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 1, "type": "filter", "hook": "forward", "prio": 0, "policy": "drop"}},
	{"chain": {"family": "ip", "table": "filter", "name": "ufw-before-forward", "handle": 2}},
	{"chain": {"family": "ip", "table": "filter", "name": "ufw-after-forward", "handle": 3}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"jump": {"target": "ufw-before-forward"}}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "ufw-before-forward", "expr": [{"match": {"left": {"ct": {"key": "state"}}, "op": "in", "right": ["established", "related"]}}, {"accept": null}]}}
]}`

const nftJSONUFWDropWithBridge = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 1, "type": "filter", "hook": "forward", "prio": 0, "policy": "drop"}},
	{"chain": {"family": "ip", "table": "filter", "name": "ufw-before-forward", "handle": 2}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"jump": {"target": "ufw-before-forward"}}]}},
	{"rule": {"family": "ip", "table": "filter", "chain": "ufw-before-forward", "expr": [{"match": {"left": {"meta": {"key": "iifname"}}, "op": "==", "right": "workshopbr0"}}, {"accept": null}]}}
]}`

const nftJSONGenericDrop = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 1, "type": "filter", "hook": "forward", "prio": 0, "policy": "drop"}},
	{"rule": {"family": "ip", "table": "filter", "chain": "FORWARD", "expr": [{"match": {"left": {"ct": {"key": "state"}}, "op": "in", "right": ["established", "related"]}}, {"accept": null}]}}
]}`

const nftJSONEmpty = `{"nftables": [
	{"chain": {"family": "ip", "table": "filter", "name": "FORWARD", "handle": 1, "type": "filter", "hook": "forward", "prio": 0, "policy": "accept"}}
]}`

// --- analyzeNftJSON tests ---

func (s *FirewallTests) TestAnalyzeNftJSONDockerDropBlocked(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONDockerDrop), "workshopbr0")
	c.Assert(msg, check.Not(check.Equals), "")
	c.Check(msg, check.Matches, ".*Docker.*")
	c.Check(msg, check.Matches, ".*DOCKER-USER.*")
	c.Check(msg, check.Matches, ".*workshopbr0.*")
}

func (s *FirewallTests) TestAnalyzeNftJSONDockerDropCompensated(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONDockerDropWithBridge), "workshopbr0")
	c.Assert(msg, check.Equals, "")
}

func (s *FirewallTests) TestAnalyzeNftJSONAcceptPolicy(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONAcceptPolicy), "workshopbr0")
	c.Assert(msg, check.Equals, "")
}

func (s *FirewallTests) TestAnalyzeNftJSONUFWBlocked(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONUFWDrop), "workshopbr0")
	c.Assert(msg, check.Not(check.Equals), "")
	c.Check(msg, check.Matches, ".*UFW.*")
	c.Check(msg, check.Matches, ".*ufw allow.*")
}

func (s *FirewallTests) TestAnalyzeNftJSONUFWCompensated(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONUFWDropWithBridge), "workshopbr0")
	c.Assert(msg, check.Equals, "")
}

func (s *FirewallTests) TestAnalyzeNftJSONGenericDrop(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONGenericDrop), "workshopbr0")
	c.Assert(msg, check.Not(check.Equals), "")
	c.Check(msg, check.Not(check.Matches), ".*Docker.*")
	c.Check(msg, check.Not(check.Matches), ".*UFW.*")
	c.Check(msg, check.Matches, ".*workshopbr0.*")
}

func (s *FirewallTests) TestAnalyzeNftJSONEmpty(c *check.C) {
	msg := analyzeNftJSON([]byte(nftJSONEmpty), "workshopbr0")
	c.Assert(msg, check.Equals, "")
}

func (s *FirewallTests) TestAnalyzeNftJSONInvalidJSON(c *check.C) {
	msg := analyzeNftJSON([]byte("not json"), "workshopbr0")
	c.Assert(msg, check.Equals, "")
}

func (s *FirewallTests) TestAnalyzeNftJSONNoForwardChain(c *check.C) {
	msg := analyzeNftJSON([]byte(`{"nftables": []}`), "workshopbr0")
	c.Assert(msg, check.Equals, "")
}

// --- bridgeBlockedWarning tests ---

func (s *FirewallTests) TestBridgeBlockedWarningDocker(c *check.C) {
	msg := bridgeBlockedWarning("workshopbr0", causeDocker)
	c.Check(msg, check.Matches, ".*Docker.*nft.*DOCKER-USER.*workshopbr0.*")
	c.Check(msg, check.Matches, ".*canonical.com/lxd/docs.*")
}

func (s *FirewallTests) TestBridgeBlockedWarningUFW(c *check.C) {
	msg := bridgeBlockedWarning("workshopbr0", causeUFW)
	c.Check(msg, check.Matches, ".*UFW.*ufw allow.*workshopbr0.*")
	c.Check(msg, check.Matches, ".*canonical.com/lxd/docs.*")
}

func (s *FirewallTests) TestBridgeBlockedWarningUnknown(c *check.C) {
	msg := bridgeBlockedWarning("workshopbr0", causeUnknown)
	c.Check(msg, check.Not(check.Matches), ".*Docker.*")
	c.Check(msg, check.Not(check.Matches), ".*UFW.*")
	c.Check(msg, check.Matches, ".*workshopbr0.*")
	c.Check(msg, check.Matches, ".*canonical.com/lxd/docs.*")
}
