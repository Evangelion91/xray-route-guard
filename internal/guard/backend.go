package guard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrConflict = errors.New("active configuration changed externally; manual recovery required")
var ErrRecovery = errors.New("unresolved recovery journal; manual attention required")

type Backend struct{ Config Config }
type journal struct {
	Previous      []byte `json:"previous"`
	CandidateHash string `json:"candidate_hash"`
}

func (b *Backend) path(name string) string { return filepath.Join(b.Config.StateDir, name) }
func (b *Backend) Prepare() error {
	for _, dir := range []string{b.Config.StateDir, b.Config.RuntimeDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return errors.New("cannot create guard directory")
		}
		st, e := os.Lstat(dir)
		if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 || !ownedByCurrentUser(st) {
			return errors.New("guard directories must be real owner-only directories")
		}
	}
	data, e := privateRead(b.Config.ActiveConfig, 4<<20)
	if e != nil {
		return errors.New("cannot read private active configuration")
	}
	if _, e = replaceOutbound(data, b.Config.OutboundTag, Node{Outbound: json.RawMessage(`{}`)}); e != nil {
		return e
	}
	return requireHealthRoute(data, b.Config)
}
func (b *Backend) CurrentID() string {
	data, e := privateRead(b.Config.ActiveConfig, 4<<20)
	if e != nil {
		return ""
	}
	return activeNodeID(data, b.Config.OutboundTag)
}
func (b *Backend) client(socks string) *http.Client {
	tr := &http.Transport{DisableKeepAlives: true, ResponseHeaderTimeout: time.Duration(b.Config.Timeout) * time.Second}
	if socks != "" {
		u := &url.URL{Scheme: "socks5", Host: socks}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, Timeout: time.Duration(b.Config.Timeout) * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}
func (b *Backend) healthy(ctx context.Context, socks string) bool {
	client := b.client(socks)
	defer client.CloseIdleConnections()
	for _, target := range b.Config.Targets {
		req, e := http.NewRequestWithContext(ctx, "GET", target.URL, nil)
		if e != nil {
			return false
		}
		response, e := client.Do(req)
		if e != nil {
			return false
		}
		response.Body.Close()
		if response.StatusCode != target.Status {
			return false
		}
	}
	return true
}
func (b *Backend) Health(ctx context.Context) bool { return b.healthy(ctx, b.Config.ActiveSOCKS) }
func (b *Backend) Refresh(ctx context.Context) ([]Node, error) {
	secret, e := privateRead(b.Config.SubscriptionFile, 8192)
	if e != nil {
		return nil, errors.New("cannot read private subscription URL")
	}
	u, e := url.Parse(strings.TrimSpace(string(secret)))
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("subscription URL must use HTTPS without userinfo")
	}
	for _, socks := range []string{"", b.Config.ActiveSOCKS} {
		client := b.client(socks)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
				return errors.New("subscription redirect rejected")
			}
			return nil
		}
		req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if e != nil {
			return nil, errors.New("invalid subscription request")
		}
		response, e := client.Do(req)
		if e != nil {
			client.CloseIdleConnections()
			continue
		}
		data, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		client.CloseIdleConnections()
		if e != nil || response.StatusCode != 200 || len(data) > 1<<20 {
			continue
		}
		nodes, e := ParseSubscription(data, b.Config.Include)
		if e != nil {
			continue
		}
		if e = writeJSON(b.path("nodes.json"), nodes); e != nil {
			return nil, errors.New("cannot persist subscription cache")
		}
		return nodes, nil
	}
	return nil, errors.New("subscription refresh failed; cached nodes retained")
}
func (b *Backend) Cached() []Node {
	data, e := privateRead(b.path("nodes.json"), 2<<20)
	if e != nil {
		return nil
	}
	var nodes []Node
	if json.Unmarshal(data, &nodes) != nil || len(nodes) > 128 {
		return nil
	}
	// Reapply the current policy on startup, even when the subscription is down.
	// Rebuild IDs instead of trusting stored identity or protocol metadata.
	var outbounds []Document
	for _, node := range nodes {
		var out Document
		if json.Unmarshal(node.Outbound, &out) != nil || out == nil {
			return nil
		}
		out["tag"], _ = json.Marshal(node.Label)
		outbounds = append(outbounds, out)
	}
	raw, _ := json.Marshal(outbounds)
	filtered, _ := ParseSubscription(raw, b.Config.Include)
	return filtered
}
func (b *Backend) validate(ctx context.Context, path string) error {
	timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if exec.CommandContext(timeout, b.Config.Xray, "run", "-test", "-config", path).Run() != nil {
		return errors.New("Xray configuration validation failed")
	}
	return nil
}
func (b *Backend) restart(ctx context.Context) error {
	timeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if exec.CommandContext(timeout, b.Config.Restart[0], b.Config.Restart[1:]...).Run() != nil {
		return errors.New("service restart failed")
	}
	return nil
}
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
func (b *Backend) Verify(ctx context.Context) error { return b.validate(ctx, b.Config.ActiveConfig) }

func (b *Backend) Probe(ctx context.Context, node Node) bool {
	addr := net.JoinHostPort("127.0.0.1", itoa(b.Config.ProbePort))
	reserved, e := net.Listen("tcp", addr)
	if e != nil {
		return false
	}
	reserved.Close()
	var out Document
	if json.Unmarshal(node.Outbound, &out) != nil {
		return false
	}
	out["tag"] = json.RawMessage(`"candidate"`)
	config := map[string]any{"log": map[string]string{"loglevel": "none"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": b.Config.ProbePort, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": false}}}, "outbounds": []any{out}}
	path := filepath.Join(b.Config.RuntimeDir, "probe.json")
	if writeJSON(path, config) != nil {
		return false
	}
	defer os.Remove(path)
	if b.validate(ctx, path) != nil {
		return false
	}
	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, b.Config.Xray, "run", "-config", path)
	if cmd.Start() != nil {
		cancel()
		return false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()
	// No PID files or pattern-based killing: only this exact child is stopped.
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		select {
		case err := <-done:
			done <- err
			return false
		default:
		}
		conn, e := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if e == nil {
			conn.Close()
			break
		}
		if !pause(ctx, 100*time.Millisecond) {
			return false
		}
	}
	for i := 0; i < 2; i++ {
		if !b.healthy(ctx, addr) {
			return false
		}
		select {
		case err := <-done:
			done <- err
			return false
		default:
		}
		if i == 0 && !pause(ctx, 2*time.Second) {
			return false
		}
	}
	return true
}

// Recover rolls back an interrupted transition only when the active file still
// belongs to that transition. An operator's concurrent edit is never overwritten.
func (b *Backend) Recover(ctx context.Context) error {
	data, e := privateRead(b.path("pending.json"), 8<<20)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return errors.New("cannot read recovery journal")
	}
	var j journal
	if json.Unmarshal(data, &j) != nil || len(j.Previous) == 0 || j.CandidateHash == "" {
		return errors.New("invalid recovery journal")
	}
	active, e := privateRead(b.Config.ActiveConfig, 4<<20)
	if e != nil {
		return errors.New("cannot inspect active configuration during recovery")
	}
	if digest(active) != j.CandidateHash && digest(active) != digest(j.Previous) {
		return ErrConflict
	}
	if e = atomicWrite(b.Config.ActiveConfig, j.Previous); e != nil {
		return errors.New("cannot restore previous configuration")
	}
	if e = b.restart(ctx); e != nil {
		return e
	}
	if e = removeDurably(b.path("pending.json")); e != nil {
		return errors.New("cannot clear recovery journal")
	}
	return nil
}

func (b *Backend) rollback(j journal, reason error) error {
	// Commit can fail after unlink but before the directory sync. Recreate
	// the journal before rollback so a second failure remains recoverable.
	if err := writeJSON(b.path("pending.json"), j); err != nil {
		return errors.Join(ErrRecovery, reason, errors.New("cannot persist rollback journal"))
	}
	recovery, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if err := b.Recover(recovery); err != nil {
		return errors.Join(ErrRecovery, reason, err)
	}
	return reason
}

func (b *Backend) Apply(ctx context.Context, node Node) error {
	if _, e := os.Lstat(b.path("pending.json")); !os.IsNotExist(e) {
		return ErrRecovery
	}
	original, e := privateRead(b.Config.ActiveConfig, 4<<20)
	if e != nil {
		return errors.New("cannot read active configuration")
	}
	candidate, e := replaceOutbound(original, b.Config.OutboundTag, node)
	if e != nil {
		return e
	}
	staged := filepath.Join(b.Config.RuntimeDir, "staged.json")
	if writeErr := atomicWrite(staged, candidate); writeErr != nil {
		return errors.New("cannot stage candidate")
	}
	defer os.Remove(staged)
	if e = b.validate(ctx, staged); e != nil {
		return e
	}
	// Include the final active health check here so callers cannot bypass it.
	if b.Health(ctx) {
		return errors.New("active route recovered; switch cancelled")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	current, e := privateRead(b.Config.ActiveConfig, 4<<20)
	if e != nil || digest(current) != digest(original) {
		return ErrConflict
	}
	j := journal{original, digest(candidate)}
	if e = writeJSON(b.path("pending.json"), j); e != nil {
		return errors.New("cannot persist recovery journal")
	}
	// Every failure from this point leaves a recoverable journal. Rollback gets
	// its own deadline even when a termination signal cancelled the main context.
	rollback := func(reason error) error { return b.rollback(j, reason) }
	if e = atomicWrite(b.Config.ActiveConfig, candidate); e != nil {
		return rollback(errors.New("cannot activate candidate"))
	}
	if e = b.restart(ctx); e != nil {
		return rollback(e)
	}
	if !pause(ctx, 2*time.Second) || !b.Health(ctx) || !pause(ctx, time.Second) || !b.Health(ctx) {
		return rollback(errors.New("post-switch health failed"))
	}
	current, e = privateRead(b.Config.ActiveConfig, 4<<20)
	if e != nil || digest(current) != digest(candidate) {
		return ErrConflict
	}
	if e = atomicWrite(b.path("previous.json"), original); e != nil {
		return rollback(errors.New("cannot persist previous configuration"))
	}
	if e = removeDurably(b.path("pending.json")); e != nil {
		return rollback(errors.New("cannot commit transition"))
	}
	return nil
}
