package guard

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The helper validates config as a child process. Actual-core tests can use
// XRAY_BINARY; no tests connect to a provider or need a real subscription.
func init() {
	if os.Getenv("GUARD_XRAY_HELPER") == "1" {
		data, e := os.ReadFile(os.Args[len(os.Args)-1])
		if e != nil || !json.Valid(data) || strings.Contains(string(data), `"invalid"`) {
			os.Exit(1)
		}
		os.Exit(0)
	}
}
func fixture(t *testing.T) *Backend {
	t.Helper()
	dir := t.TempDir()
	binary, _ := os.Executable()
	b := &Backend{Config: Config{ActiveConfig: filepath.Join(dir, "active.json"), Xray: binary, OutboundTag: "managed", ActiveSOCKS: "127.0.0.1:1080", StateDir: filepath.Join(dir, "state"), RuntimeDir: filepath.Join(dir, "runtime"), Restart: []string{"/bin/true"}, Timeout: 1, ProbePort: 32129}}
	original := []byte(`{"log":{"loglevel":"warning"},"inbounds":[{"tag":"stay","protocol":"socks","listen":"127.0.0.1","port":1080}],"routing":{"rules":[{"type":"field","inboundTag":["stay"],"outboundTag":"managed"}]},"outbounds":[{"tag":"managed","protocol":"blackhole","settings":{}},{"tag":"untouched","protocol":"freedom"}]}`)
	if e := os.WriteFile(b.Config.ActiveConfig, original, 0600); e != nil {
		t.Fatal(e)
	}
	if e := b.Prepare(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("GUARD_XRAY_HELPER", "1")
	return b
}
func socksServer(t *testing.T) string {
	t.Helper()
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { ln.Close(); wg.Wait() })
	go func() {
		for {
			conn, e := ln.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				header := make([]byte, 2)
				if _, e := io.ReadFull(conn, header); e != nil {
					return
				}
				methods := make([]byte, int(header[1]))
				if _, e = io.ReadFull(conn, methods); e != nil {
					return
				}
				conn.Write([]byte{5, 0})
				request := make([]byte, 4)
				if _, e = io.ReadFull(conn, request); e != nil {
					return
				}
				var host string
				switch request[3] {
				case 1:
					ip := make([]byte, 4)
					if _, e = io.ReadFull(conn, ip); e != nil {
						return
					}
					host = net.IP(ip).String()
				case 3:
					n := make([]byte, 1)
					if _, e = io.ReadFull(conn, n); e != nil {
						return
					}
					name := make([]byte, int(n[0]))
					if _, e = io.ReadFull(conn, name); e != nil {
						return
					}
					host = string(name)
				case 4:
					ip := make([]byte, 16)
					if _, e = io.ReadFull(conn, ip); e != nil {
						return
					}
					host = net.IP(ip).String()
				default:
					return
				}
				p := make([]byte, 2)
				if _, e = io.ReadFull(conn, p); e != nil {
					return
				}
				target, e := net.DialTimeout("tcp", net.JoinHostPort(host, itoa(int(binary.BigEndian.Uint16(p)))), time.Second)
				if e != nil {
					return
				}
				defer target.Close()
				conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				copied := make(chan struct{})
				go func() { io.Copy(target, conn); close(copied) }()
				io.Copy(conn, target)
				conn.Close()
				<-copied
			}()
		}
	}()
	return ln.Addr().String()
}
func testNode() Node {
	return Node{ID: "candidate", Outbound: json.RawMessage(`{"protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":9}]}}`)}
}

func TestReplacePreservesUnrelatedConfiguration(t *testing.T) {
	b := fixture(t)
	original, _ := os.ReadFile(b.Config.ActiveConfig)
	changed, e := replaceOutbound(original, "managed", testNode())
	if e != nil {
		t.Fatal(e)
	}
	var before, after Document
	json.Unmarshal(original, &before)
	json.Unmarshal(changed, &after)
	for _, k := range []string{"inbounds", "routing", "log"} {
		var a, z any
		json.Unmarshal(before[k], &a)
		json.Unmarshal(after[k], &z)
		av, _ := json.Marshal(a)
		zv, _ := json.Marshal(z)
		if string(av) != string(zv) {
			t.Fatal("changed", k)
		}
	}
	if !strings.Contains(string(changed), "untouched") {
		t.Fatal("lost unrelated outbound")
	}
	duplicate := []byte(`{"outbounds":[{"tag":"managed"},{"tag":"managed"}]}`)
	if _, e = replaceOutbound(duplicate, "managed", testNode()); e == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestApplyPostcheckAndRollback(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(itoa(map[bool]int{true: 1, false: 0}[success]), func(t *testing.T) {
			b := fixture(t)
			original, _ := os.ReadFile(b.Config.ActiveConfig)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := os.ReadFile(b.Config.ActiveConfig)
				if success && activeNodeID(data, "managed") != activeNodeID(original, "managed") {
					w.WriteHeader(204)
				} else {
					w.WriteHeader(503)
				}
			}))
			defer server.Close()
			b.Config.Targets = []Target{{server.URL, 204}}
			b.Config.ActiveSOCKS = socksServer(t)
			err := b.Apply(context.Background(), testNode())
			after, _ := os.ReadFile(b.Config.ActiveConfig)
			if success {
				if err != nil || string(after) == string(original) {
					t.Fatalf("commit failed %v", err)
				}
			} else {
				if err == nil || string(after) != string(original) {
					t.Fatalf("rollback failed %v", err)
				}
			}
			if _, e := os.Stat(b.path("pending.json")); !os.IsNotExist(e) {
				t.Fatal("journal left after resolved transition")
			}
		})
	}
}
func TestCrashRecoveryAndExternalEdit(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(itoa(map[bool]int{true: 1, false: 0}[edited]), func(t *testing.T) {
			b := fixture(t)
			old, _ := os.ReadFile(b.Config.ActiveConfig)
			candidate, _ := replaceOutbound(old, "managed", testNode())
			writeJSON(b.path("pending.json"), journal{old, digest(candidate)})
			if edited {
				candidate = []byte(`{"operator":"changed"}`)
			}
			atomicWrite(b.Config.ActiveConfig, candidate)
			err := b.Recover(context.Background())
			after, _ := os.ReadFile(b.Config.ActiveConfig)
			if edited {
				if !errors.Is(err, ErrConflict) || string(after) != string(candidate) {
					t.Fatal("external edit overwritten")
				}
			} else {
				if err != nil || string(after) != string(old) {
					t.Fatal("recovery failed", err)
				}
			}
		})
	}
}
func TestFailedRecoveryCannotOverwriteJournal(t *testing.T) {
	b := fixture(t)
	original, _ := os.ReadFile(b.Config.ActiveConfig)
	j := journal{original, "pending"}
	writeJSON(b.path("pending.json"), j)
	if err := b.Apply(context.Background(), testNode()); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(b.path("pending.json"))
	var after journal
	json.Unmarshal(data, &after)
	if after.CandidateHash != "pending" {
		t.Fatal("lost recovery journal")
	}
}
func TestLockNotInheritedByChild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard.lock")
	a, e := Lock(path)
	if e != nil {
		t.Fatal(e)
	}
	if b, e := Lock(path); e == nil {
		b.Close()
		t.Fatal("double lock")
	}
	child := exec.Command("sleep", "1")
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	defer child.Wait()
	a.Close()
	b, e := Lock(path)
	if e != nil {
		t.Fatal("child inherited lock", e)
	}
	b.Close()
}
func TestOccupiedProbePortUntouched(t *testing.T) {
	b := fixture(t)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	b.Config.ProbePort = ln.Addr().(*net.TCPAddr).Port
	if b.Probe(context.Background(), testNode()) {
		t.Fatal("occupied port accepted")
	}
	conn, e := net.Dial("tcp", ln.Addr().String())
	if e != nil {
		t.Fatal("foreign listener stopped")
	}
	conn.Close()
}
func TestRealXrayProbe(t *testing.T) {
	binaryPath := os.Getenv("XRAY_BINARY")
	if binaryPath == "" {
		t.Skip("set XRAY_BINARY for isolated real-core integration")
	}
	b := fixture(t)
	t.Setenv("GUARD_XRAY_HELPER", "")
	b.Config.Xray = binaryPath
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	b.Config.Targets = []Target{{server.URL, 204}}
	address := socksServer(t)
	host, port, _ := net.SplitHostPort(address)
	var node Node
	node.Outbound = json.RawMessage(`{"protocol":"socks","settings":{"servers":[{"address":"` + host + `","port":` + port + `}]}}`)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	b.Config.ProbePort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if !b.Probe(context.Background(), node) {
		t.Fatal("real-core probe failed")
	}
	reused, e := net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(b.Config.ProbePort)))
	if e != nil {
		t.Fatal("probe process leaked")
	}
	reused.Close()
}
