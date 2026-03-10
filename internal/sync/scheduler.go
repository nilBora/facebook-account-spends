package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"facebook-account-parser/internal/db"
)

// Scheduler wraps cron and runs the pipeline on two schedules:
//   - 10:00 AM daily   — discover accounts + sync yesterday's spend
//   - every 2 hours    — sync today's spend
type Scheduler struct {
	cron     *cron.Cron
	pipeline *Pipeline
	store    db.Store
}

// NewScheduler creates a Scheduler with two configurable cron jobs.
func NewScheduler(pipeline *Pipeline, store db.Store, scheduleYesterday, scheduleToday string) (*Scheduler, error) {
	c := cron.New()
	s := &Scheduler{cron: c, pipeline: pipeline, store: store}

	// Morning job: rediscover accounts and sync the previous day.
	// Runs in the morning so attribution-lag data from yesterday is settled.
	_, err := c.AddFunc(scheduleYesterday, func() {
		ctx := context.Background()
		yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		slog.Info("scheduler: morning sync started", "date", yesterday)

		runID, _ := s.store.CreateSyncRun(ctx, "cron_yesterday", yesterday)

		if err := pipeline.DiscoverAccounts(ctx); err != nil {
			slog.Error("scheduler: discovery failed", "err", err)
		}
		res := pipeline.SyncDate(ctx, yesterday)

		if runID != "" {
			_ = s.store.FinishSyncRun(ctx, runID, res.Success, len(res.Errors))
			if len(res.Errors) > 0 {
				_ = s.store.AddSyncErrors(ctx, runID, res.Errors)
			}
		}
		slog.Info("scheduler: morning sync done", "date", yesterday,
			"success", res.Success, "errors", len(res.Errors))
	})
	if err != nil {
		return nil, err
	}

	// Today job: sync today's spend for live tracking.
	_, err = c.AddFunc(scheduleToday, func() {
		ctx := context.Background()
		today := time.Now().UTC().Format("2006-01-02")
		slog.Info("scheduler: today sync started", "date", today)

		runID, _ := s.store.CreateSyncRun(ctx, "cron_today", today)
		res := pipeline.SyncDate(ctx, today)

		if runID != "" {
			_ = s.store.FinishSyncRun(ctx, runID, res.Success, len(res.Errors))
			if len(res.Errors) > 0 {
				_ = s.store.AddSyncErrors(ctx, runID, res.Errors)
			}
		}
		slog.Info("scheduler: today sync done", "date", today,
			"success", res.Success, "errors", len(res.Errors))
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
	for _, e := range s.cron.Entries() {
		slog.Info("scheduler: job registered", "next_run", e.Next.Format("2006-01-02 15:04:05"))
	}
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
