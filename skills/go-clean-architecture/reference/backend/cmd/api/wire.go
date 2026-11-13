package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"example.com/app/internal/api"
	leadshttp "example.com/app/internal/leads/delivery/httpapi"
	leadspg "example.com/app/internal/leads/repository/postgres"
	leadsuc "example.com/app/internal/leads/usecase"
	"example.com/app/internal/platform/clock"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/httpserver"
	"example.com/app/internal/platform/jobs"
)

// newHandler mounts the probes and the /v1 API with the middleware every API request goes
// through, outermost first. main and the API test both call it.
func newHandler(log *slog.Logger, v1 http.Handler, health *httpserver.Health, c cfg) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health.Live) // probes skip the middleware: no access log lines, no spans
	mux.HandleFunc("GET /readyz", health.Ready)
	mux.Handle("/v1/", httpserver.Observe(httpserver.Chain(v1,
		httpserver.RequestID,      // so every log line of the request has the ID (and Observe's trace ID)
		httpserver.APIHeaders,     // no-store, nosniff on every answer, problems included
		httpserver.AccessLog(log), // one line per request, with the final status
		httpserver.Recover(log),   // a panic becomes a logged 500
		httpserver.Timeout(c.HandlerTimeout),
		httpserver.LimitBody,
		http.NewCrossOriginProtection().Handler, // cookie-authenticated mutations from other sites
	), serverName(c.Addr), httpserver.NewClientIP(c.TrustedProxies)))
	return mux
}

// serverName is the fixed server.address:server.port the request metrics report.
func serverName(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "api"
	}
	return net.JoinHostPort("api", port)
}

// newJobsClient is the API's insert-only River client: no queues, never started; the worker
// process works the jobs. It carries the trace context into the jobs it inserts.
func newJobsClient(pool *pgxpool.Pool, log *slog.Logger) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "river", Logger: log, Middleware: jobs.Middleware(),
	})
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}
	return client, nil
}

// newAPI wires each module's storage → use cases → HTTP adapter, and returns the /v1 API.
// main and the API test both call it, so the test runs what production runs. Aliases name a
// module's layer packages: <module><layer> (leadspg, leadsuc, leadshttp).
func newAPI(log *slog.Logger, pool *pgxpool.Pool, bus *events.Bus) (http.Handler, error) {
	leadsStore := leadspg.NewStore(pool, bus)
	leads := leadshttp.New(leadshttp.UseCases{
		Create: leadsuc.NewCreateLead(leadsStore, clock.System{}, uuid.NewV7),
		Get:    leadsuc.NewGetLead(leadsStore.Queries()),
		Assign: leadsuc.NewAssignLead(leadsStore.Leads()),
	})

	// The generated StrictServerInterface is satisfied by embedding every module's handler.
	return api.New(struct{ *leadshttp.Handler }{leads}, log)
}
