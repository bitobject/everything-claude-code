package main

import (
	"log/slog"
	"net/http"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/app/internal/api"
	leadshttp "example.com/app/internal/leads/delivery/httpapi"
	leadspg "example.com/app/internal/leads/repository/postgres"
	leadsuc "example.com/app/internal/leads/usecase"
	"example.com/app/internal/platform/clock"
	"example.com/app/internal/platform/events"
)

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
