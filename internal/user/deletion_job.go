package user

import (
	"context"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/shared/port"
)

const (
	deletionJobFirstDelay = 2 * time.Minute
	deletionJobInterval   = 24 * time.Hour
)

// DeletionJob runs the daily deletion passes in-process. Only serve starts it.
type DeletionJob struct {
	svc   *Service
	clock port.Clock
}

func NewDeletionJob(svc *Service, clock port.Clock) *DeletionJob {
	return &DeletionJob{svc: svc, clock: clock}
}

// StartPolling always starts, whatever the settings: the release pass matters
// even with inactivity switched off. One goroutine, so runs never overlap.
func (j *DeletionJob) StartPolling(ctx context.Context) {
	go func() {
		timer := time.NewTimer(deletionJobFirstDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		j.RunOnce(ctx)
		ticker := time.NewTicker(deletionJobInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				j.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce releases first, so a partner who came back saves the other before
// any wipe is tried; marking comes last.
func (j *DeletionJob) RunOnce(ctx context.Context) DeletionPassResult {
	res := j.svc.PurgeDeleted(ctx)
	if res.Err != nil {
		slog.Error("deletion: pass failed", "step", "purge", "err_type", failureReason(res.Err))
	}
	marked, err := j.svc.MarkInactive(ctx)
	if err != nil {
		slog.Error("deletion: pass failed", "step", "mark", "err_type", failureReason(err))
		if res.Err == nil {
			res.Err = err
		}
	}
	res.Marked = marked
	slog.Info("deletion: pass done", "released", res.Released, "wiped", res.Wiped, "failed", len(res.Failed), "marked", res.Marked)
	return res
}
