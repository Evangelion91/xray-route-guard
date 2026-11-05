package guard

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRealXrayVLESSConfigurationMatrix(t *testing.T) {
	binary := os.Getenv("XRAY_BINARY")
	if binary == "" {
		t.Skip("set XRAY_BINARY for installed-core validation")
	}
	b := fixture(t)
	t.Setenv("GUARD_XRAY_HELPER", "")
	b.Config.Xray = binary
	// Public, synthetic test values; no connection is made by run -test.
	key := base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	for name, query := range map[string]string{
		"tcp-tls":       "type=tcp&security=tls&sni=example.com",
		"ws-tls":        "type=ws&security=tls&sni=example.com&path=%2Fws&host=example.com",
		"grpc-tls":      "type=grpc&security=tls&sni=example.com&serviceName=test&mode=gun",
		"xhttp-tls":     "type=xhttp&security=tls&sni=example.com&path=%2Fx&mode=auto",
		"tcp-reality":   "type=tcp&security=reality&sni=example.com&pbk=" + key + "&sid=0123456789abcdef&flow=xtls-rprx-vision",
		"xhttp-reality": "type=xhttp&security=reality&sni=example.com&pbk=" + key + "&sid=0123456789abcdef&path=%2Fx&mode=auto",
	} {
		t.Run(name, func(t *testing.T) {
			link := "vless://00000000-0000-4000-8000-000000000001@example.com:443?" + query + "#synthetic"
			nodes, err := ParseSubscription([]byte(link), "")
			if err != nil {
				t.Fatal(err)
			}
			config := map[string]any{"log": map[string]string{"loglevel": "none"}, "outbounds": []json.RawMessage{nodes[0].Outbound}}
			path := filepath.Join(b.Config.RuntimeDir, "matrix.json")
			if err := writeJSON(path, config); err != nil {
				t.Fatal(err)
			}
			if err := b.validate(context.Background(), path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
