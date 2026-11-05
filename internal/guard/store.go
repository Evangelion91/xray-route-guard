package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

func itoa(n int) string      { return strconv.Itoa(n) }
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func atomicWrite(path string, data []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".guard-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func writeJSON(path string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(path, append(b, '\n'))
}
func removeDurably(path string) error {
	if e := os.Remove(path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func SaveStatus(dir string, status Status) error {
	return writeJSON(filepath.Join(dir, "status.json"), status)
}

type Document map[string]json.RawMessage

func replaceOutbound(original []byte, tag string, node Node) ([]byte, error) {
	var doc Document
	if json.Unmarshal(original, &doc) != nil || doc == nil {
		return nil, errors.New("active configuration must be plain JSON")
	}
	var out []Document
	if json.Unmarshal(doc["outbounds"], &out) != nil {
		return nil, errors.New("outbounds missing")
	}
	matches := 0
	for i, item := range out {
		var current string
		json.Unmarshal(item["tag"], &current)
		if current == tag {
			matches++
			var replacement Document
			if json.Unmarshal(node.Outbound, &replacement) != nil || replacement == nil {
				return nil, errors.New("invalid outbound")
			}
			replacement["tag"], _ = json.Marshal(tag)
			out[i] = replacement
		}
	}
	if matches != 1 {
		return nil, errors.New("outbound_tag must match exactly one outbound")
	}
	doc["outbounds"], _ = json.Marshal(out)
	return json.MarshalIndent(doc, "", "  ")
}
func activeNodeID(data []byte, tag string) string {
	var doc struct {
		Outbounds []Document `json:"outbounds"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return ""
	}
	for _, o := range doc.Outbounds {
		var t string
		json.Unmarshal(o["tag"], &t)
		if t == tag {
			delete(o, "tag")
			b, _ := json.Marshal(o)
			return digest(b)
		}
	}
	return ""
}
