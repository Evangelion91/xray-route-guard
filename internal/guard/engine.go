package guard

import (
	"context"
	"errors"
	"time"
)

type Driver interface {
	Health(context.Context) bool
	Refresh(context.Context) ([]Node, error)
	Probe(context.Context, Node) bool
	Apply(context.Context, Node) error
}
type Status struct {
	Observed   time.Time `json:"observed_at"`
	Healthy    bool      `json:"healthy"`
	Failures   int       `json:"consecutive_failures"`
	Event      string    `json:"event"`
	Selected   string    `json:"selected_id,omitempty"`
	Candidates int       `json:"cached_candidates"`
}
type Engine struct {
	Driver         Driver
	Config         Config
	Nodes          []Node
	Status         Status
	LastRefresh    time.Time
	LastScan       time.Time
	LastRefreshTry time.Time
}

func (e *Engine) Tick(ctx context.Context, now time.Time) error {
	s := &e.Status
	s.Observed = now
	s.Healthy = e.Driver.Health(ctx)
	s.Event = "healthy"
	if s.Healthy {
		s.Failures = 0
	} else {
		s.Failures++
		s.Event = "waiting_for_threshold"
	}
	due := e.LastRefresh.IsZero() || now.Sub(e.LastRefresh) >= time.Duration(e.Config.RefreshInterval)*time.Second || (!s.Healthy && s.Failures == 1)
	if due && (e.LastRefreshTry.IsZero() || now.Sub(e.LastRefreshTry) >= time.Minute) {
		e.LastRefreshTry = now
		if nodes, err := e.Driver.Refresh(ctx); err == nil {
			e.Nodes = nodes
			e.LastRefresh = now
		} else {
			s.Event = "refresh_failed_cache_retained"
		}
	}
	s.Candidates = len(e.Nodes)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.Healthy || s.Failures < e.Config.FailureThreshold {
		return nil
	}
	if !e.LastScan.IsZero() && now.Sub(e.LastScan) < time.Duration(e.Config.Cooldown)*time.Second {
		s.Event = "cooldown"
		return nil
	}
	scanStart := time.Now()
	defer func() { e.LastScan = now.Add(time.Since(scanStart)) }()
	s.Event = "no_verified_replacement"
	for _, node := range e.Nodes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if node.ID == s.Selected {
			continue
		}
		if !e.Driver.Probe(ctx, node) {
			continue
		}
		if e.Driver.Health(ctx) {
			s.Failures = 0
			s.Healthy = true
			s.Event = "recovered_switch_cancelled"
			return nil
		}
		if err := e.Driver.Apply(ctx, node); err == nil {
			s.Selected = node.ID
			s.Failures = 0
			s.Healthy = true
			s.Event = "switched"
			return nil
		} else if errors.Is(err, ErrConflict) || errors.Is(err, ErrRecovery) {
			s.Event = "manual_recovery_required"
			return err
		} else {
			s.Event = "switch_failed_previous_retained"
		}
	}
	return nil
}
