package guard

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const testLink = "vless://00000000-0000-4000-8000-000000000001@[::1]:443?type=ws&security=tls&sni=example.com&host=example.com&path=%2Fsocket#Europe"

func TestSubscriptionFormatsAndFilter(t *testing.T) {
	for _, body := range []string{testLink, base64.StdEncoding.EncodeToString([]byte(testLink)), base64.RawURLEncoding.EncodeToString([]byte(testLink)), testLink + "\n" + testLink} {
		nodes, e := ParseSubscription([]byte(body), "^Europe$")
		if e != nil || len(nodes) != 1 {
			t.Fatalf("%v %d", e, len(nodes))
		}
		var v struct {
			Settings struct {
				Vnext []struct {
					Address string
					Port    int
				}
			}
			StreamSettings struct{ WSSettings struct{ Path string } }
		}
		json.Unmarshal(nodes[0].Outbound, &v)
		if v.Settings.Vnext[0].Address != "::1" || v.StreamSettings.WSSettings.Path != "/socket" {
			t.Fatalf("bad parsing: %+v", v)
		}
	}
	if _, e := ParseSubscription([]byte(testLink), "^Other$"); e == nil {
		t.Fatal("empty filter result accepted")
	}
}
func TestRejectUnsafeAndUnsupportedLinks(t *testing.T) {
	for _, body := range []string{"<html>maintenance</html>", strings.Replace(testLink, "security=tls", "security=none", 1), testLink + "&unknown=x", strings.Replace(testLink, "#Europe", "&allowInsecure=1#Europe", 1), strings.Replace(testLink, "#Europe", "&security=tls#Europe", 1), `[{"protocol":"freedom","tag":"direct"}]`, `[{"protocol":"vless","proxySettings":{"tag":"current"}}]`, `[{"protocol":"vless","streamSettings":{"sockopt":{"dialerProxy":"current"}}}]`} {
		// Unknown fragment text is a label, not a query parameter.
		if strings.HasSuffix(body, "&unknown=x") {
			body = strings.Replace(body, "#Europe&unknown=x", "&unknown=x#Europe", 1)
		}
		if _, e := ParseSubscription([]byte(body), ""); e == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestRealityXHTTP(t *testing.T) {
	link := strings.Replace(testLink, "type=ws&security=tls", "type=xhttp&security=reality", 1)
	link = strings.Replace(link, "#Europe", "&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=01&mode=stream-up#Europe", 1)
	nodes, e := ParseSubscription([]byte(link), "")
	if e != nil {
		t.Fatal(e)
	}
	var n map[string]any
	json.Unmarshal(nodes[0].Outbound, &n)
	stream := n["streamSettings"].(map[string]any)
	if stream["xhttpSettings"].(map[string]any)["mode"] != "stream-up" || stream["realitySettings"].(map[string]any)["fingerprint"] != "chrome" {
		t.Fatal(stream)
	}
}
func FuzzSubscription(f *testing.F) {
	f.Add([]byte(testLink))
	f.Add([]byte("[]"))
	f.Fuzz(func(t *testing.T, b []byte) {
		nodes, _ := ParseSubscription(b, "")
		for _, n := range nodes {
			if !json.Valid(n.Outbound) || len(n.ID) != 64 {
				t.Fatal("invalid accepted node")
			}
		}
	})
}
