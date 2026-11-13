// Package notifications is a thin module (STACK.md, "Module profiles"): one package with its
// job, its service code and its sqlc queries, its own schema, and no rules beyond formatting.
// It reacts to leads.LeadCreated by recording a message for the operators' feed. It knows the
// leads module only through leads/contract.
package notifications

import (
	"context"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	leads "example.com/app/internal/leads/contract"
	"example.com/app/internal/notifications/sqlcgen"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/jobs"
	"example.com/app/internal/platform/postgres"
)

// LeadCreatedArgs is the job a leads.LeadCreated event becomes for this module. River keeps it
// as JSON for days (retries over about three weeks, then retention), so it is the one place
// with json tags outside the adapters: fields are only ever added, never renamed (a renamed
// field decodes to its zero value in jobs already queued), it carries IDs only (no personal
// data in the job tables), and TestLeadCreatedArgsJSON pins the encoding.
type LeadCreatedArgs struct {
	LeadID uuid.UUID `json:"lead_id"`
}

// Kind names the job; never change it once deployed.
func (LeadCreatedArgs) Kind() string { return "notifications.lead_created" }

// Subscribe registers the module's subscriptions. cmd calls it in every process that publishes.
func Subscribe(bus *events.Bus) {
	events.Subscribe(bus, func(e leads.LeadCreated) river.JobArgs { return LeadCreatedArgs{LeadID: e.LeadID} })
}

// Worker records one message per new lead.
type Worker struct {
	river.WorkerDefaults[LeadCreatedArgs]
	leads leads.Reader
	q     *sqlcgen.Queries
}

// NewWorker returns the worker; cmd/worker registers it with River.
func NewWorker(pool *pgxpool.Pool, reader leads.Reader) *Worker {
	return &Worker{leads: reader, q: sqlcgen.New(pool)}
}

// Work reads the lead through the leads module's facade and records the message. It may run
// more than once for one job: the insert ignores a lead that already has its message.
func (w *Worker) Work(ctx context.Context, job *river.Job[LeadCreatedArgs]) error {
	lead, err := w.leads.Lead(ctx, job.Args.LeadID)
	if err != nil {
		return jobs.Outcome(fmt.Errorf("read lead %s: %w", job.Args.LeadID, err))
	}
	err = w.q.InsertMessage(ctx, sqlcgen.InsertMessageParams{
		ID:     uuid.NewV7(),
		LeadID: lead.ID,
		Body:   "New lead: " + lead.Topic + ", " + lead.City,
	})
	if err != nil {
		return jobs.Outcome(fmt.Errorf("insert message: %w", postgres.MapError(err)))
	}
	return nil
}
