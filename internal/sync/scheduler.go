package sync

import (
	"context"
	"log/slog"

	"github.com/robfig/cron/v3"
)

// Scheduler wraps cron and runs the pipeline on a schedule.
type Scheduler struct {
	cron     *cron.Cron
	pipeline *Pipeline
}

// NewScheduler creates a Scheduler that runs the pipeline on the given cron expression.
func NewScheduler(pipeline *Pipeline, schedule string) (*Scheduler, error) {
	c := cron.New()

	_, err := c.AddFunc(schedule, func() {
		slog.Info("scheduler triggered daily sync")
		pipeline.RunDaily(context.Background())
	})
	if err != nil {
		return nil, err
	}

	return &Scheduler{cron: c, pipeline: pipeline}, nil
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	slog.Info("scheduler started")
	s.cron.Start()
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
