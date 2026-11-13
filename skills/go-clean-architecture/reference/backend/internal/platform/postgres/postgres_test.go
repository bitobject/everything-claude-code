package postgres_test

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"example.com/app/internal/platform/postgres"
	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/internal/platform/telemetry/telemetrytest"
	"example.com/app/migrations"
)

func TestQueryName(t *testing.T) {
	tests := []struct{ name, sql, want string }{
		{name: "sqlc query", sql: "-- name: CreateLead :one\nINSERT INTO leads.leads (id) VALUES ($1)", want: "CreateLead"},
		{name: "sqlc query after whitespace", sql: "\n  -- name: GetLead :one\nSELECT 1", want: "GetLead"},
		{name: "plain statement", sql: "select count(*) from river.river_job", want: "SELECT"},
		{name: "another comment first", sql: "-- a note\nUPDATE x SET y = 1", want: "UPDATE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := postgres.QueryName(t.Context(), tt.sql); got != tt.want {
				t.Errorf("QueryName(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

// A query inside a traced request gets a span named by its sqlc name, with the statement text and
// never the argument values; a query outside any trace (River polling) gets none.
func TestQuerySpansKeepArgumentsOut(t *testing.T) {
	rec := telemetrytest.Install(t) // before the pool: the tracer binds to the providers it finds
	pool := pgtest.New(t, migrations.FS)
	const phone = "+79991234567"
	const sql = "-- name: EchoPhone :one\nSELECT $1::text"

	var got string
	if err := pool.QueryRow(t.Context(), sql, phone).Scan(&got); err != nil { // no parent span
		t.Fatal(err)
	}
	ctx, parent := otel.Tracer("test").Start(t.Context(), "request")
	if err := pool.QueryRow(ctx, sql, phone).Scan(&got); err != nil {
		t.Fatal(err)
	}
	parent.End()

	spans := rec.SpansNamed("EchoPhone")
	if len(spans) != 1 {
		t.Fatalf("EchoPhone spans = %d of %v, want 1 (only the traced call)", len(spans), telemetrytest.Names(rec.Spans()))
	}
	if text, _ := telemetrytest.Attr(spans[0].Attributes(), semconv.DBQueryTextKey); !strings.Contains(text, "$1") {
		t.Errorf("db.query.text = %q, want the statement with its placeholder", text)
	}
	for _, kv := range spans[0].Attributes() {
		if strings.Contains(kv.Value.String(), "79991234567") {
			t.Errorf("span attribute %s = %q carries an argument value", kv.Key, kv.Value.String())
		}
	}
	found := false
	for _, set := range rec.Series(t, "db.client.operation.duration") {
		if v, ok := set.Value(semconv.DBOperationNameKey); ok && v.AsString() == "EchoPhone" {
			found = true
		}
	}
	if !found {
		t.Error("no db.client.operation.duration series for EchoPhone")
	}
}
