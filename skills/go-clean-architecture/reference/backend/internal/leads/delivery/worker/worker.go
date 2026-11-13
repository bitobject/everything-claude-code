// Package worker is the leads module's job adapter: the River jobs the module runs and their
// schedules. A job becomes a use-case call here, as a request does in delivery/httpapi; the
// rules stay in the use case.
package worker

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"example.com/app/internal/leads/usecase"
	"example.com/app/internal/platform/jobs"
)

// PurgeIdempotencyKeysArgs is the periodic purge job. It has no fields; jobs already queued
// must still decode after a deploy, so fields are only ever added (TestPurgeArgsJSON).
type PurgeIdempotencyKeysArgs struct{}

// Kind names the job; never change it once deployed.
func (PurgeIdempotencyKeysArgs) Kind() string { return "leads.purge_idempotency_keys" }

// PurgeIdempotencyKeys runs usecase.PurgeIdempotencyKeys.
type PurgeIdempotencyKeys struct {
	river.WorkerDefaults[PurgeIdempotencyKeysArgs]
	purge *usecase.PurgeIdempotencyKeys
}

// NewPurgeIdempotencyKeys returns the worker; cmd/worker registers it.
func NewPurgeIdempotencyKeys(purge *usecase.PurgeIdempotencyKeys) *PurgeIdempotencyKeys {
	return &PurgeIdempotencyKeys{purge: purge}
}

// Work deletes the expired keys. A storage error is returned, so River retries the job.
func (w *PurgeIdempotencyKeys) Work(ctx context.Context, _ *river.Job[PurgeIdempotencyKeysArgs]) error {
	_, err := w.purge.Handle(ctx)
	return jobs.Outcome(err)
}

// PeriodicJobs returns the module's schedules; cmd/worker passes them to its River client, whose
// elected leader inserts the jobs. The purge runs hourly and once at start (RunOnStart), so a
// worker that restarts often still purges: the leader keeps its schedule only in memory.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return PurgeIdempotencyKeysArgs{}, nil },
			&river.PeriodicJobOpts{ID: "leads.purge_idempotency_keys", RunOnStart: true},
		),
	}
}
