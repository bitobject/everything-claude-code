package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/app/internal/api"
	"example.com/app/internal/api/apitest"
	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/notifications"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/httpserver"
	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/internal/platform/telemetry/telemetrytest"
	"example.com/app/migrations"
)

// TestAPIOnPostgres runs requests through the stack main builds (newHandler, newAPI,
// newJobsClient): middleware, router, validator, handlers, use cases, sqlc, PostgreSQL as the
// application role, and the subscriber's job, which continues the request's trace.
func TestAPIOnPostgres(t *testing.T) {
	telemetrytest.Install(t) // before the pool and the clients, as telemetry.Setup in main
	pool := pgtest.New(t, migrations.FS)
	log := slog.New(slog.DiscardHandler)
	client, err := newJobsClient(pool, log)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus(client)
	notifications.Subscribe(bus)
	v1, err := newAPI(log, pool, bus)
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(log, v1, httpserver.NewHealth(pool.Ping), cfg{Addr: ":8080", HandlerTimeout: 10 * time.Second})
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		apitest.CheckContract(t, req, rec)
		for k, want := range map[string]string{"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff"} {
			if got := rec.Header().Get(k); got != want {
				t.Errorf("%s %s: %s = %q, want %q", req.Method, req.URL.Path, k, got, want)
			}
		}
		return rec
	}

	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/leads",
			strings.NewReader(`{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", `"0b8f2a6c-1d4e-4f7a-9c3b-5e6d7f8a9b0c"`)
		req.Header.Set("Traceparent", "00-"+traceID+"-00f067aa0ba902b7-01") // as Next.js sends it
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

	var traceparents []string
	rows, err := pool.Query(t.Context(), `SELECT coalesce(metadata->>'traceparent', '') FROM river.river_job WHERE kind = $1 AND args->>'lead_id' = $2`,
		notifications.LeadCreatedArgs{}.Kind(), lead.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var tp string
		if err := rows.Scan(&tp); err != nil {
			t.Fatal(err)
		}
		traceparents = append(traceparents, tp)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(traceparents) != 1 {
		t.Fatalf("notifications jobs for the lead = %d, want 1 (the repeat must not publish again)", len(traceparents))
	}
	if !strings.HasPrefix(traceparents[0], "00-"+traceID+"-") {
		t.Errorf("job traceparent = %q, want the request's trace %s (the worker's span links to it)", traceparents[0], traceID)
	}
}

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
