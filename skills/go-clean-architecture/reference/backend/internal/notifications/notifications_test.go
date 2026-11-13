package notifications_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	leads "example.com/app/internal/leads/contract"
	"example.com/app/internal/notifications"
	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/migrations"
)

// TestLeadCreatedArgsJSON pins the job's encoding: jobs already in the queue must still decode
// after a deploy. Change this test only by adding fields.
func TestLeadCreatedArgsJSON(t *testing.T) {
	args := notifications.LeadCreatedArgs{LeadID: uuid.MustParse("019a7a3e-0000-7000-8000-000000000001")}
	got, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	const golden = `{"lead_id":"019a7a3e-0000-7000-8000-000000000001"}`
	if string(got) != golden {
		t.Errorf("encoding = %s, want %s", got, golden)
	}
	var back notifications.LeadCreatedArgs
	if err := json.Unmarshal([]byte(golden), &back); err != nil || back != args {
		t.Errorf("decoding the golden JSON = %+v, %v; want %+v", back, err, args)
	}
}

type fakeReader struct {
	lead leads.Lead
	err  error
}

func (f fakeReader) Lead(context.Context, uuid.UUID) (leads.Lead, error) { return f.lead, f.err }

func job(id uuid.UUID) *river.Job[notifications.LeadCreatedArgs] {
	return &river.Job[notifications.LeadCreatedArgs]{JobRow: &rivertype.JobRow{}, Args: notifications.LeadCreatedArgs{LeadID: id}}
}

// TestWorkCancelsWhenTheLeadIsGone: retrying can't bring a deleted lead back, so the job is
// cancelled instead of being retried for weeks.
func TestWorkCancelsWhenTheLeadIsGone(t *testing.T) {
	w := notifications.NewWorker(nil, fakeReader{err: leads.ErrLeadNotFound})
	err := w.Work(t.Context(), job(uuid.NewV7()))
	if _, ok := errors.AsType[*rivertype.JobCancelError](err); !ok {
		t.Errorf("Work() = %v, want a JobCancel error", err)
	}
}

func TestWorkRetriesOtherErrors(t *testing.T) {
	boom := errors.New("connection refused")
	w := notifications.NewWorker(nil, fakeReader{err: boom})
	err := w.Work(t.Context(), job(uuid.NewV7()))
	if _, ok := errors.AsType[*rivertype.JobCancelError](err); ok || !errors.Is(err, boom) {
		t.Errorf("Work() = %v, want %v returned for a retry", err, boom)
	}
}

func TestWorkRecordsOneMessagePerLead(t *testing.T) {
	pool := pgtest.New(t, migrations.FS)
	lead := leads.Lead{ID: uuid.NewV7(), Topic: "Divorce", City: "Kazan", Status: "new", CreatedAt: time.Now().UTC()}
	w := notifications.NewWorker(pool, fakeReader{lead: lead})
	for range 2 { // a job may run twice
		if err := w.Work(t.Context(), job(lead.ID)); err != nil {
			t.Fatalf("Work() = %v", err)
		}
	}
	var n int
	var body string
	if err := pool.QueryRow(t.Context(), `SELECT count(*), min(body) FROM notifications.messages WHERE lead_id = $1`, lead.ID).Scan(&n, &body); err != nil {
		t.Fatal(err)
	}
	if n != 1 || body != "New lead: Divorce, Kazan" {
		t.Errorf("messages = %d %q, want 1 \"New lead: Divorce, Kazan\"", n, body)
	}
}
