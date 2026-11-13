package postgres

import (
	"context"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/app/internal/leads/repository/postgres/sqlcgen"
	"example.com/app/internal/leads/usecase"
)

// Queries is the read side: sqlc rows straight into views, without the aggregate.
type Queries struct{ q *sqlcgen.Queries }

var _ usecase.LeadQueries = Queries{}

// NewQueries returns the read side on pool. A process that only reads (the worker serving the
// module's facade) needs nothing else.
func NewQueries(pool *pgxpool.Pool) Queries { return Queries{q: sqlcgen.New(pool)} }

// Lead returns the view of the lead with id, or usecase.ErrLeadNotFound.
func (q Queries) Lead(ctx context.Context, id uuid.UUID) (usecase.LeadView, error) {
	row, err := q.q.GetLeadView(ctx, id)
	if err != nil {
		return usecase.LeadView{}, mapError(err, "select lead view")
	}
	return usecase.LeadView{
		ID:        row.ID,
		Topic:     row.Topic,
		City:      row.City,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
	}, nil
}
