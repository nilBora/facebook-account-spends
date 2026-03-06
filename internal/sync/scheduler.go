package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler wraps cron and runs the pipeline on two schedules:
//   - 10:00 AM daily   — discover accounts + sync yesterday's spend
//   - every 2 hours    — sync today's spend
type Scheduler struct {
	cron     *cron.Cron
	pipeline *Pipeline
}

// NewScheduler creates a Scheduler with two configurable cron jobs.
func NewScheduler(pipeline *Pipeline, scheduleYesterday, scheduleToday string) (*Scheduler, error) {
	c := cron.New()

	// Morning job: rediscover accounts and sync the previous day.
	// Runs in the morning so attribution-lag data from yesterday is settled.
	_, err := c.AddFunc(scheduleYesterday, func() {
		ctx := context.Background()
		yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		slog.Info("scheduler: morning sync started", "date", yesterday)

		if err := pipeline.DiscoverAccounts(ctx); err != nil {
			slog.Error("scheduler: discovery failed", "err", err)
		}
		if err := pipeline.SyncDate(ctx, yesterday); err != nil {
			slog.Error("scheduler: yesterday sync failed", "date", yesterday, "err", err)
		}
		slog.Info("scheduler: morning sync done", "date", yesterday)
	})
	if err != nil {
		return nil, err
	}

	// Today job: sync today's spend for live tracking.
	_, err = c.AddFunc(scheduleToday, func() {
		ctx := context.Background()
		today := time.Now().UTC().Format("2006-01-02")
		slog.Info("scheduler: today sync started", "date", today)

		if err := pipeline.SyncDate(ctx, today); err != nil {
			slog.Error("scheduler: today sync failed", "date", today, "err", err)
		}
		slog.Info("scheduler: today sync done", "date", today)
	})
	if err != nil {
		return nil, err
	}

	return &Scheduler{cron: c, pipeline: pipeline}, nil
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	entries := s.cron.Entries()
	for _, e := range entries {
		slog.Info("scheduler: job registered", "next_run", e.Next.Format("2006-01-02 15:04:05"))
	}
	s.cron.Start()
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
