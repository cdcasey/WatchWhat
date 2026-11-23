package catalog

import (
	"context"
	"log/slog"
	"time"
)

// Refresher periodically refreshes airing shows the household is tracking.
type Refresher struct {
	Svc          *Service
	Log          *slog.Logger
	InitialDelay time.Duration // default 30s
	Interval     time.Duration // default 6h
	MaxAge       time.Duration // default 24h
}

func (r *Refresher) defaults() {
	if r.InitialDelay == 0 {
		r.InitialDelay = 30 * time.Second
	}
	if r.Interval == 0 {
		r.Interval = 6 * time.Hour
	}
	if r.MaxAge == 0 {
		r.MaxAge = 24 * time.Hour
	}
	if r.Log == nil {
		r.Log = slog.Default()
	}
}

// Run blocks until ctx is cancelled.
func (r *Refresher) Run(ctx context.Context) {
	r.defaults()
	t := time.NewTimer(r.InitialDelay)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		r.RunOnce(ctx)
		t.Reset(r.Interval)
	}
}

// RunOnce refreshes every eligible show sequentially and returns how many succeeded.
// Failures are logged; the show stays eligible for the next run.
func (r *Refresher) RunOnce(ctx context.Context) int {
	r.defaults()
	cutoff := r.Svc.Now().Add(-r.MaxAge).Unix()
	shows, err := r.Svc.Q.ListRefreshCandidates(ctx, cutoff)
	if err != nil {
		r.Log.Warn("refresh: listing candidates", "err", err)
		return 0
	}
	ok := 0
	for _, s := range shows {
		if ctx.Err() != nil {
			break
		}
		if err := r.Svc.Refresh(ctx, s.ID); err != nil {
			r.Log.Warn("refresh failed", "media_id", s.ID, "title", s.Title, "err", err)
			continue
		}
		ok++
	}
	if len(shows) > 0 {
		r.Log.Info("refresh run", "eligible", len(shows), "refreshed", ok)
	}
	return ok
}
