package guard

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeDriver struct {
	health                     []bool
	refreshErr                 error
	applyErr                   error
	probes, applies, refreshes int
	probeOK                    bool
	nodes                      []Node
}

func (f *fakeDriver) Health(context.Context) bool {
	if len(f.health) == 0 {
		return false
	}
	v := f.health[0]
	f.health = f.health[1:]
	return v
}
func (f *fakeDriver) Refresh(context.Context) ([]Node, error) {
	f.refreshes++
	return f.nodes, f.refreshErr
}
func (f *fakeDriver) Probe(context.Context, Node) bool  { f.probes++; return f.probeOK }
func (f *fakeDriver) Apply(context.Context, Node) error { f.applies++; return f.applyErr }
func testEngine(f *fakeDriver) *Engine {
	return &Engine{Driver: f, Config: Config{FailureThreshold: 3, Cooldown: 120, RefreshInterval: 21600}, Nodes: []Node{{ID: "backup"}}}
}
func TestTransientDoesNotSwitch(t *testing.T) {
	f := &fakeDriver{health: []bool{false, false, true}, refreshErr: errors.New("offline"), probeOK: true}
	e := testEngine(f)
	for i := 0; i < 3; i++ {
		if err := e.Tick(context.Background(), time.Unix(int64(1000+i*30), 0)); err != nil {
			t.Fatal(err)
		}
	}
	if f.probes != 0 || f.applies != 0 || e.Status.Failures != 0 || len(e.Nodes) != 1 {
		t.Fatalf("unexpected transition: %+v %+v", f, e.Status)
	}
}
func TestVerifiedCandidateAndRecoveryCancellation(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(itoa(map[bool]int{false: 0, true: 1}[recovered]), func(t *testing.T) {
			f := &fakeDriver{health: []bool{false, false, false, recovered}, probeOK: true, nodes: []Node{{ID: "backup"}}}
			e := testEngine(f)
			for i := 0; i < 3; i++ {
				if err := e.Tick(context.Background(), time.Unix(int64(1000+i*30), 0)); err != nil {
					t.Fatal(err)
				}
			}
			expected := 1
			if recovered {
				expected = 0
			}
			if f.applies != expected || f.probes != 1 || !e.Status.Healthy {
				t.Fatalf("bad switch: %+v %+v", f, e.Status)
			}
		})
	}
}
func TestAllDownKeepsCacheAndBacksOff(t *testing.T) {
	f := &fakeDriver{refreshErr: errors.New("offline")}
	e := testEngine(f)
	e.Status.Failures = 2
	for i := 0; i < 3; i++ {
		if err := e.Tick(context.Background(), time.Unix(int64(1000+i*30), 0)); err != nil {
			t.Fatal(err)
		}
	}
	if f.probes != 1 || f.applies != 0 || len(e.Nodes) != 1 || e.Status.Event != "cooldown" {
		t.Fatalf("unexpected: %+v %+v", f, e.Status)
	}
}
func TestUnresolvedJournalStopsFurtherCandidates(t *testing.T) {
	for _, failure := range []error{ErrConflict, ErrRecovery} {
		f := &fakeDriver{probeOK: true, applyErr: failure, nodes: []Node{{ID: "a"}, {ID: "b"}}}
		e := testEngine(f)
		e.Status.Failures = 2
		if err := e.Tick(context.Background(), time.Unix(1000, 0)); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if f.applies != 1 || f.probes != 1 {
			t.Fatal("must halt after unresolved recovery")
		}
	}
}
func TestNeverReprobesCurrentNode(t *testing.T) {
	f := &fakeDriver{nodes: []Node{{ID: "current"}}, probeOK: true}
	e := testEngine(f)
	e.Status.Selected = "current"
	e.Status.Failures = 2
	if err := e.Tick(context.Background(), time.Unix(1000, 0)); err != nil {
		t.Fatal(err)
	}
	if f.probes != 0 || f.applies != 0 {
		t.Fatal("current node was selected as replacement")
	}
}
