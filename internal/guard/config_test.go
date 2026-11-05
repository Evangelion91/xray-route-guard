package guard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPrivateAndStrict(t *testing.T) {
	raw, e := os.ReadFile("../../config.example.json")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, raw, 0600)
	if _, e = LoadConfig(path); e != nil {
		t.Fatal(e)
	}
	os.Chmod(path, 0644)
	if _, e = LoadConfig(path); e == nil {
		t.Fatal("public-readable config accepted")
	}
	os.Chmod(path, 0600)
	var original map[string]any
	json.Unmarshal(raw, &original)
	for key, value := range map[string]any{"unknown": true, "active_socks": "0.0.0.0:1080", "probe_port": 1080, "timeout_seconds": -1, "include": "[", "targets": []Target{{"http://example.com", 200}}} {
		var c map[string]any
		json.Unmarshal(raw, &c)
		c[key] = value
		data, _ := json.Marshal(c)
		os.WriteFile(path, data, 0600)
		if _, e = LoadConfig(path); e == nil {
			t.Fatal("accepted invalid", key)
		}
	}
}
func TestRejectHealthBypass(t *testing.T) {
	b := fixture(t)
	raw, _ := os.ReadFile(b.Config.ActiveConfig)
	if e := requireHealthRoute(raw, b.Config); e != nil {
		t.Fatal(e)
	}
	for _, changed := range []string{strings.Replace(string(raw), `"outboundTag":"managed"`, `"outboundTag":"untouched"`, 1), strings.Replace(string(raw), `"listen":"127.0.0.1"`, `"listen":"0.0.0.0"`, 1), strings.Replace(string(raw), `"type":"field"`, `"type":"field","domain":["example.com"]`, 1)} {
		if requireHealthRoute([]byte(changed), b.Config) == nil {
			t.Fatal("ambiguous health route accepted")
		}
	}
}
