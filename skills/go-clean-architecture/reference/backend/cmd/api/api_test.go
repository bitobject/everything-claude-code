package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"example.com/app/internal/api"
	"example.com/app/internal/api/apitest"
	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/notifications"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/migrations"
)

// TestAPIOnPostgres runs requests through the stack main builds (newAPI): router, validator,
// handlers, use cases, sqlc, PostgreSQL as the application role, and the subscriber's job.
func TestAPIOnPostgres(t *testing.T) {
	pool := pgtest.New(t, migrations.FS)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "river"})
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus(client)
	notifications.Subscribe(bus)
	h, err := newAPI(slog.New(slog.DiscardHandler), pool, bus)
	if err != nil {
		t.Fatal(err)
	}
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		apitest.CheckContract(t, req, rec)
		return rec
	}

	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/leads",
			strings.NewReader(`{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", `"0b8f2a6c-1d4e-4f7a-9c3b-5e6d7f8a9b0c"`)
		return do(req)
	}
	first, again := create(), create()
	if first.Code != http.StatusCreated || again.Body.String() != first.Body.String() {
		t.Fatalf("create = %d %s; repeat = %d %s; want 201 and the same lead", first.Code, first.Body, again.Code, again.Body)
	}
	var lead openapi.Lead
	if err := json.Unmarshal(first.Body.Bytes(), &lead); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/leads/"+lead.Id.String()+"/assign", nil)
	req.AddCookie(&http.Cookie{Name: api.SessionCookie, Value: "s"})
	if rec := do(req); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"assigned"`) {
		t.Fatalf("assign = %d %s, want 200 assigned", rec.Code, rec.Body)
	}

	var jobs int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river.river_job WHERE kind = $1 AND args->>'lead_id' = $2`,
		notifications.LeadCreatedArgs{}.Kind(), lead.Id.String()).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Errorf("notifications jobs for the lead = %d, want 1 (the repeat must not publish again)", jobs)
	}
}
