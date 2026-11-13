// Command worker is the composition root of the background jobs: it registers every module's
// River workers and runs them until SIGTERM, then stops gracefully: no new jobs, running jobs
// get SHUTDOWN_GRACE to finish, then their contexts are cancelled.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	leadsfacade "example.com/app/internal/leads/delivery/facade"
	leadsworker "example.com/app/internal/leads/delivery/worker"
	leadspg "example.com/app/internal/leads/repository/postgres"
	leadsuc "example.com/app/internal/leads/usecase"
	"example.com/app/internal/notifications"
	"example.com/app/internal/platform/clock"
	"example.com/app/internal/platform/config"
	"example.com/app/internal/platform/jobs"
	"example.com/app/internal/platform/logging"
	"example.com/app/internal/platform/postgres"
	"example.com/app/internal/platform/telemetry"
)

type cfg struct {
	DatabaseURL config.Secret `env:"DATABASE_URL,required,unset"` // the application role
	MaxWorkers  int           `env:"WORKER_CONCURRENCY" envDefault:"10"`
	Grace       time.Duration `env:"SHUTDOWN_GRACE" envDefault:"30s"`
	LogLevel    slog.Level    `env:"LOG_LEVEL" envDefault:"INFO"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var c cfg
	if err := env.Parse(&c); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logging.New(os.Stdout, c.LogLevel)

	// Before the pool and the client: their instrumentation binds to the providers it installs.
	shutdownTelemetry, err := telemetry.Setup(ctx, "worker", log)
	if err != nil {
		return err
	}
	defer telemetry.Flush(ctx, log, shutdownTelemetry) // runs last: after the jobs have stopped

	pool, err := postgres.Open(ctx, c.DatabaseURL.Reveal(), postgres.DefaultConfig("worker"))
	if err != nil {
		return err
	}
	defer pool.Close()

	client, err := newClient(pool, log, c.MaxWorkers)
	if err != nil {
		return err
	}
	// Start gets a context that SIGTERM doesn't cancel: cancelling it would cancel the running
	// jobs at once. The stop below is soft first.
	if err := client.Start(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("start jobs: %w", err)
	}
	log.InfoContext(ctx, "working jobs")
	<-ctx.Done()

	soft, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.Grace)
	defer cancel()
	if err := client.Stop(soft); err == nil {
		return nil
	} else if !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("stop jobs: %w", err)
	}
	log.WarnContext(ctx, "jobs still running after the grace period; cancelling them")
	hard, cancelHard := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelHard()
	if err := client.StopAndCancel(hard); err != nil {
		return fmt.Errorf("stop jobs: %w", err)
	}
	return nil
}

// newClient registers every module's workers and periodic jobs (each module's
// delivery/worker.PeriodicJobs) with one River client.
func newClient(pool *pgxpool.Pool, log *slog.Logger, maxWorkers int) (*river.Client[pgx.Tx], error) {
	// Module facades other modules' workers call.
	leads := leadsfacade.New(leadsuc.NewGetLead(leadspg.NewQueries(pool)))

	workers := river.NewWorkers()
	river.AddWorker(workers, notifications.NewWorker(pool, leads))
	river.AddWorker(workers, leadsworker.NewPurgeIdempotencyKeys(
		leadsuc.NewPurgeIdempotencyKeys(leadspg.NewKeyPurger(pool), clock.System{})))

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema:       "river",
		Logger:       log,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: maxWorkers}},
		Workers:      workers,
		PeriodicJobs: leadsworker.PeriodicJobs(), // slices.Concat the next module's
		Middleware:   jobs.Middleware(),          // a span per job, linked to the request that enqueued it
	})
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}
	return client, nil
}
