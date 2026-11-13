package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/migrations"
)

// TestPeriodicPurgeRunsOnStart checks the wiring of cmd/worker on PostgreSQL: the client starts
// under the application role, becomes leader, inserts the purge job (RunOnStart) and works it.
func TestPeriodicPurgeRunsOnStart(t *testing.T) {
	pool := pgtest.New(t, migrations.FS)
	client, err := newClient(pool, slog.New(slog.DiscardHandler), 2)
	if err != nil {
		t.Fatal(err)
	}
	completed, unsubscribe := client.Subscribe(river.EventKindJobCompleted)
	defer unsubscribe()
	// As in main: Start's context is never cancelled, the stop is explicit.
	if err := client.Start(context.WithoutCancel(t.Context())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		if err := client.Stop(ctx); err != nil {
			t.Errorf("Stop() = %v", err)
		}
	})

	timeout := time.After(30 * time.Second)
	for {
		select {
		case e := <-completed:
			if e.Job.Kind == "leads.purge_idempotency_keys" {
				return
			}
		case <-timeout:
			t.Fatal("no leads.purge_idempotency_keys job completed within 30s of start")
		}
	}
}
