package guard

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
)

// A successful request is useful evidence only if it traverses the managed
// outbound. Require a dedicated SOCKS inbound and an unconditional first rule;
// otherwise a direct/default route could hide a broken proxy indefinitely.
func requireHealthRoute(data []byte, c Config) error {
	bad := errors.New("health SOCKS inbound needs an unconditional first routing rule to outbound_tag; see docs/configuration.md")
	var doc struct {
		Inbounds []Document `json:"inbounds"`
		Routing  struct {
			Rules []Document `json:"rules"`
		} `json:"routing"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return bad
	}
	host, port, _ := net.SplitHostPort(c.ActiveSOCKS)
	number, _ := strconv.Atoi(port)
	inboundTag := ""
	matches := 0
	for _, in := range doc.Inbounds {
		var p int
		var text string
		if json.Unmarshal(in["port"], &p) != nil {
			json.Unmarshal(in["port"], &text)
			p, _ = strconv.Atoi(text)
		}
		if p != number {
			continue
		}
		var protocol, listen, tag string
		json.Unmarshal(in["protocol"], &protocol)
		json.Unmarshal(in["listen"], &listen)
		json.Unmarshal(in["tag"], &tag)
		if protocol != "socks" || listen != host || tag == "" {
			return bad
		}
		var settings struct {
			Auth string `json:"auth"`
		}
		if len(in["settings"]) > 0 && json.Unmarshal(in["settings"], &settings) != nil {
			return bad
		}
		if settings.Auth != "" && settings.Auth != "noauth" {
			return bad
		}
		matches++
		inboundTag = tag
	}
	if matches != 1 || len(doc.Routing.Rules) == 0 {
		return bad
	}
	first := doc.Routing.Rules[0]
	var tags []string
	var target, kind string
	if len(first) != 3 || json.Unmarshal(first["inboundTag"], &tags) != nil || json.Unmarshal(first["outboundTag"], &target) != nil || json.Unmarshal(first["type"], &kind) != nil {
		return bad
	}
	if kind != "field" || target != c.OutboundTag || len(tags) != 1 || tags[0] != inboundTag {
		return bad
	}
	return nil
}
