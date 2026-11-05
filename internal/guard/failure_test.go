package guard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRollbackAfterJournalWasUnlinked(t *testing.T) {
	b := fixture(t)
	original, _ := os.ReadFile(b.Config.ActiveConfig)
	candidate, _ := replaceOutbound(original, "managed", testNode())
	if err := atomicWrite(b.Config.ActiveConfig, candidate); err != nil {
		t.Fatal(err)
	}
	// A directory-sync failure after unlink leaves this exact on-disk state.
	reason := errors.New("commit durability failed")
	if err := b.rollback(journal{original, digest(candidate)}, reason); err != reason {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(b.Config.ActiveConfig)
	if string(after) != string(original) {
		t.Fatal("previous configuration not restored")
	}
	if _, err := os.Stat(b.path("pending.json")); !os.IsNotExist(err) {
		t.Fatal("resolved journal retained")
	}
}

func TestFailedRollbackRemainsRecoverable(t *testing.T) {
	b := fixture(t)
	original, _ := os.ReadFile(b.Config.ActiveConfig)
	candidate, _ := replaceOutbound(original, "managed", testNode())
	atomicWrite(b.Config.ActiveConfig, candidate)
	b.Config.Restart = []string{"/bin/false"}
	if err := b.rollback(journal{original, digest(candidate)}, errors.New("postcheck failed")); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	if _, err := privateRead(b.path("pending.json"), 8<<20); err != nil {
		t.Fatal("lost journal", err)
	}
	b.Config.Restart = []string{"/bin/true"}
	if err := b.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCachedNodesRespectChangedPolicy(t *testing.T) {
	b := fixture(t)
	a, z := testNode(), testNode()
	a.Label = "allowed"
	z.Label = "excluded"
	z.Outbound = json.RawMessage(`{"protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":10}]}}`)
	if err := writeJSON(b.path("nodes.json"), []Node{a, z}); err != nil {
		t.Fatal(err)
	}
	b.Config.Include = "^allowed$"
	nodes := b.Cached()
	if len(nodes) != 1 || nodes[0].Label != "allowed" || nodes[0].ID == a.ID {
		t.Fatal("cache policy or identity not refreshed")
	}
	b.Config.Include = "^none$"
	if len(b.Cached()) != 0 {
		t.Fatal("excluded stale nodes reused")
	}
}

func TestPrivateReadRejectsSymlinksAndSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	os.WriteFile(file, []byte("test"), 0600)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := privateRead(link, 64); err == nil {
		t.Fatal("symlink accepted")
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := privateRead(fifo, 64); err == nil {
		t.Fatal("FIFO accepted")
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(file, 65534, 65534); err != nil {
			t.Fatal(err)
		}
		if _, err := privateRead(file, 64); err == nil {
			t.Fatal("another owner's file accepted")
		}
	}
}
