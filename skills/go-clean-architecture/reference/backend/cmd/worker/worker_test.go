package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel"

	leadsworker "example.com/app/internal/leads/delivery/worker"
	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/internal/platform/telemetry/telemetrytest"
	"example.com/app/migrations"
)

// TestPeriodicPurgeRunsOnStart checks the wiring of cmd/worker on PostgreSQL: the client starts
// under the application role, becomes leader, inserts the purge job (RunOnStart) and works it.
func TestPeriodicPurgeRunsOnStart(t *testing.T) {
	_, completed := start(t)

	waitFor(t, completed, func(e *river.Event) bool { return e.Job.Kind == leadsworker.PurgeIdempotencyKeysArgs{}.Kind() })
}

// TestJobSpanLinksToTheEnqueuer: a job inserted inside a trace (a request in the API) is worked
// in a span river.work/<kind> with a link into that trace (to the river.insert_many span under
// the request). The work span starts a trace of its own: a job may run hours later.
func TestJobSpanLinksToTheEnqueuer(t *testing.T) {
	rec := telemetrytest.Install(t) // before the client, as telemetry.Setup in main
	client, completed := start(t)

	ctx, enqueuer := otel.Tracer("test").Start(t.Context(), "request")
	res, err := client.Insert(ctx, leadsworker.PurgeIdempotencyKeysArgs{}, nil)
	enqueuer.End()
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, completed, func(e *river.Event) bool { return e.Job.ID == res.Job.ID })

	for _, s := range rec.SpansNamed("river.work/" + leadsworker.PurgeIdempotencyKeysArgs{}.Kind()) {
		for _, l := range s.Links() {
			if l.SpanContext.TraceID() == enqueuer.SpanContext().TraceID() {
				return
			}
		}
	}
	t.Errorf("no work span links to the enqueuing trace; spans: %v", telemetrytest.Names(rec.Spans()))
}

// start runs the worker's client on a fresh database until the test ends and returns it with
// its completed-job events.
func start(t *testing.T) (*river.Client[pgx.Tx], <-chan *river.Event) {
	t.Helper()
	pool := pgtest.New(t, migrations.FS)
	client, err := newClient(pool, slog.New(slog.DiscardHandler), 2)
	if err != nil {
		t.Fatal(err)
	}
	completed, unsubscribe := client.Subscribe(river.EventKindJobCompleted)
	t.Cleanup(unsubscribe)
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
	return client, completed
}

func waitFor(t *testing.T, events <-chan *river.Event, match func(*river.Event) bool) {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case e := <-events:
			if match(e) {
				return
			}
		case <-timeout:
			t.Fatal("the job didn't complete within 30s")
		}
	}
}
