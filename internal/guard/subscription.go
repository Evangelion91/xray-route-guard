package guard

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Node struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Outbound json.RawMessage `json:"outbound"`
}

var uuidPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

// ParseSubscription accepts VLESS links (plain/base64) or an array of standalone
// Xray outbounds. JSON is the extension boundary for protocols not handled here.
func ParseSubscription(data []byte, include string) ([]Node, error) {
	if len(data) > 1<<20 {
		return nil, errors.New("subscription exceeds 1 MiB")
	}
	text := strings.TrimSpace(string(data))
	var values []Document
	if strings.HasPrefix(text, "[") {
		if json.Unmarshal([]byte(text), &values) != nil {
			return nil, errors.New("invalid outbound array")
		}
	} else {
		if !strings.Contains(text, "vless://") {
			compact := strings.Join(strings.Fields(text), "")
			for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
				if raw, e := enc.DecodeString(compact); e == nil {
					text = string(raw)
					break
				}
			}
		}
		for _, line := range strings.Split(text, "\n") {
			if o, e := parseVLESS(strings.TrimSpace(line)); e == nil {
				values = append(values, o)
			}
		}
	}
	filter, e := regexp.Compile(include)
	if e != nil {
		return nil, errors.New("invalid label filter")
	}
	seen := map[string]bool{}
	nodes := []Node{}
	for _, o := range values {
		var label, protocol string
		json.Unmarshal(o["tag"], &label)
		json.Unmarshal(o["protocol"], &protocol)
		if !filter.MatchString(label) {
			continue
		}
		switch protocol {
		case "vless", "vmess", "trojan", "shadowsocks", "socks", "http", "wireguard":
		default:
			continue
		}
		// Candidate must be self-contained. A dialer/proxy chain could accidentally
		// test the current route instead of the candidate, so reject such imports.
		if _, ok := o["proxySettings"]; ok {
			continue
		}
		var stream struct {
			Sockopt map[string]any `json:"sockopt"`
		}
		json.Unmarshal(o["streamSettings"], &stream)
		if _, ok := stream.Sockopt["dialerProxy"]; ok {
			continue
		}
		delete(o, "tag")
		b, _ := json.Marshal(o)
		id := digest(b)
		if !seen[id] {
			nodes = append(nodes, Node{id, label, b})
			seen[id] = true
		}
		if len(nodes) > 128 {
			return nil, errors.New("subscription exceeds 128 eligible nodes")
		}
	}
	if len(nodes) == 0 {
		return nil, errors.New("subscription contains no eligible standalone nodes")
	}
	return nodes, nil
}

func parseVLESS(link string) (Document, error) {
	bad := errors.New("unsupported VLESS link")
	u, e := url.Parse(link)
	if e != nil || u.Scheme != "vless" || u.User == nil || !uuidPattern.MatchString(u.User.Username()) || u.Hostname() == "" || u.Path != "" {
		return nil, bad
	}
	if _, ok := u.User.Password(); ok {
		return nil, bad
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return nil, bad
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return nil, bad
	}
	allowed := map[string]bool{}
	for _, k := range strings.Fields("encryption flow type security sni fp alpn pbk sid spx path host serviceName mode authority allowInsecure headerType") {
		allowed[k] = true
	}
	for k, v := range q {
		if !allowed[k] || len(v) != 1 {
			return nil, bad
		}
	}
	if v := q.Get("encryption"); v != "" && v != "none" {
		return nil, bad
	}
	if v := q.Get("allowInsecure"); v != "" && v != "0" && v != "false" {
		return nil, bad
	}
	if v := q.Get("headerType"); v != "" && v != "none" {
		return nil, bad
	}
	transport := q.Get("type")
	if transport == "" || transport == "raw" {
		transport = "tcp"
	}
	security := q.Get("security")
	if security != "tls" && security != "reality" {
		return nil, bad
	}
	stream := map[string]any{"network": transport, "security": security}
	switch transport {
	case "tcp":
	case "ws":
		stream["wsSettings"] = map[string]any{"path": q.Get("path"), "headers": map[string]string{"Host": q.Get("host")}}
	case "grpc":
		if v := q.Get("mode"); v != "" && v != "gun" && v != "multi" {
			return nil, bad
		}
		stream["grpcSettings"] = map[string]any{"serviceName": q.Get("serviceName"), "authority": q.Get("authority"), "multiMode": q.Get("mode") == "multi"}
	case "xhttp":
		mode := q.Get("mode")
		if mode == "" {
			mode = "auto"
		}
		if mode != "auto" && mode != "packet-up" && mode != "stream-up" && mode != "stream-one" {
			return nil, bad
		}
		stream["xhttpSettings"] = map[string]any{"path": q.Get("path"), "host": q.Get("host"), "mode": mode}
	default:
		return nil, bad
	}
	tls := map[string]any{}
	if q.Get("sni") != "" {
		tls["serverName"] = q.Get("sni")
	}
	if q.Get("fp") != "" {
		tls["fingerprint"] = q.Get("fp")
	}
	if security == "reality" {
		if q.Get("pbk") == "" || q.Get("sni") == "" || transport == "ws" {
			return nil, bad
		}
		tls["publicKey"] = q.Get("pbk")
		tls["shortId"] = q.Get("sid")
		if q.Get("spx") != "" {
			tls["spiderX"] = q.Get("spx")
		}
		if q.Get("fp") == "" {
			tls["fingerprint"] = "chrome"
		}
		stream["realitySettings"] = tls
	} else {
		if q.Get("alpn") != "" {
			tls["alpn"] = strings.Split(q.Get("alpn"), ",")
		}
		stream["tlsSettings"] = tls
	}
	user := map[string]any{"id": u.User.Username(), "encryption": "none"}
	if q.Get("flow") != "" {
		user["flow"] = q.Get("flow")
	}
	object := map[string]any{"tag": u.Fragment, "protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": u.Hostname(), "port": port, "users": []any{user}}}}, "streamSettings": stream}
	b, _ := json.Marshal(object)
	var out Document
	json.Unmarshal(b, &out)
	return out, nil
}
