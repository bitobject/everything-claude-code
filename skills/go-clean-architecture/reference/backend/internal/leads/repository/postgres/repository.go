// Package postgres is the leads module's persistence adapter. It implements the usecase ports
// (UnitOfWork, LeadRepository, IdempotencyKeys, IdempotencyKeyPurger, LeadQueries) with
// sqlc-generated queries on pgx, maps rows to entities and views, and translates storage facts
// into the ports' errors. The rest of the module never sees pgx or SQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/app/internal/leads/entity"
	"example.com/app/internal/leads/repository/postgres/sqlcgen"
	"example.com/app/internal/leads/usecase"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/postgres"
)

// Store is the module's storage: a unit of work plus the repositories outside one.
type Store struct {
	pool *pgxpool.Pool
	bus  *events.Bus
}

var _ usecase.UnitOfWork = (*Store)(nil)

// NewStore returns the store on pool; events published in a unit of work go through bus.
func NewStore(pool *pgxpool.Pool, bus *events.Bus) *Store { return &Store{pool: pool, bus: bus} }

// Do runs fn in one transaction with repositories bound to it.
func (s *Store) Do(ctx context.Context, fn func(context.Context, usecase.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(ctx, txRepos{tx: tx, bus: s.bus})
	})
}

// Leads returns the repository outside a unit of work: Update opens its own transaction.
func (s *Store) Leads() usecase.LeadRepository { return &LeadRepository{pool: s.pool} }

// Queries returns the read side.
func (s *Store) Queries() usecase.LeadQueries { return NewQueries(s.pool) }

// PurgeIdempotencyKeys implements usecase.IdempotencyKeyPurger (see KeyPurger).
func (s *Store) PurgeIdempotencyKeys(ctx context.Context, createdBefore time.Time) (int64, error) {
	return NewKeyPurger(s.pool).PurgeIdempotencyKeys(ctx, createdBefore)
}

// txRepos builds the repositories of one transaction. They run their statements in it directly:
// no savepoints (each costs two round trips and, at scale, subtransaction trouble).
type txRepos struct {
	tx  pgx.Tx
	bus *events.Bus
}

func (r txRepos) Leads() usecase.LeadRepository { return &LeadRepository{tx: r.tx} }

func (r txRepos) IdempotencyKeys() usecase.IdempotencyKeys {
	return idempotencyKeys{q: sqlcgen.New(r.tx)}
}

func (r txRepos) Events() usecase.Publisher { return publisher(r) }

type publisher struct {
	tx  pgx.Tx
	bus *events.Bus
}

func (p publisher) Publish(ctx context.Context, e usecase.Event) error {
	return p.bus.Publish(ctx, p.tx, e)
}

// LeadRepository stores the Lead aggregate in leads.leads, on the pool or, inside a unit of
// work, on its transaction (tx set).
type LeadRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

var _ usecase.LeadRepository = (*LeadRepository)(nil)

func (r *LeadRepository) queries() *sqlcgen.Queries {
	if r.tx != nil {
		return sqlcgen.New(r.tx)
	}
	return sqlcgen.New(r.pool)
}

// Create inserts a new lead.
func (r *LeadRepository) Create(ctx context.Context, l *entity.Lead) error {
	err := r.queries().InsertLead(ctx, sqlcgen.InsertLeadParams{
		ID:        l.ID(),
		Phone:     l.Phone().E164(),
		Topic:     l.Topic().String(),
		City:      l.City().String(),
		Status:    string(l.Status()),
		CreatedAt: l.CreatedAt(),
	})
	return mapError(err, "insert lead")
}

// Get returns the lead with id, or usecase.ErrLeadNotFound.
func (r *LeadRepository) Get(ctx context.Context, id uuid.UUID) (*entity.Lead, error) {
	row, err := r.queries().GetLead(ctx, id)
	if err != nil {
		return nil, mapError(err, "select lead")
	}
	return toEntity(row)
}

// Update locks the row, lets fn change the aggregate and writes every changeable column back,
// in one transaction (its own, or the unit of work's). fn's error rolls back and is returned
// unchanged.
func (r *LeadRepository) Update(ctx context.Context, id uuid.UUID, fn func(*entity.Lead) error) (*entity.Lead, error) {
	var lead *entity.Lead
	update := func(q *sqlcgen.Queries) error {
		row, err := q.GetLeadForUpdate(ctx, id)
		if err != nil {
			return mapError(err, "select lead for update")
		}
		l, err := toEntity(row)
		if err != nil {
			return err
		}
		if err := fn(l); err != nil {
			return err
		}
		err = q.UpdateLead(ctx, sqlcgen.UpdateLeadParams{
			ID:     l.ID(),
			Phone:  l.Phone().E164(),
			Topic:  l.Topic().String(),
			City:   l.City().String(),
			Status: string(l.Status()),
		})
		if err != nil {
			return mapError(err, "update lead")
		}
		lead = l
		return nil
	}
	if r.tx != nil {
		return lead, update(sqlcgen.New(r.tx))
	}
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error { return update(sqlcgen.New(tx)) })
	return lead, err
}

// CountCreatedSince counts the leads from phone created at or after since.
func (r *LeadRepository) CountCreatedSince(ctx context.Context, phone entity.Phone, since time.Time) (int, error) {
	n, err := r.queries().CountLeadsByPhoneSince(ctx, sqlcgen.CountLeadsByPhoneSinceParams{Phone: phone.E164(), Since: since})
	if err != nil {
		return 0, mapError(err, "count leads")
	}
	return int(n), nil
}

// toEntity rebuilds the aggregate. It re-parses the value objects, so a corrupt row fails
// loudly instead of spreading.
func toEntity(row sqlcgen.LeadsLead) (*entity.Lead, error) {
	phone, err := entity.ParsePhone(row.Phone)
	if err != nil {
		return nil, corrupt(row.ID, "phone", err)
	}
	topic, err := entity.ParseTopic(row.Topic)
	if err != nil {
		return nil, corrupt(row.ID, "topic", err)
	}
	city, err := entity.ParseCity(row.City)
	if err != nil {
		return nil, corrupt(row.ID, "city", err)
	}
	status, err := entity.ParseStatus(row.Status)
	if err != nil {
		return nil, corrupt(row.ID, "status", err)
	}
	return entity.RestoreLead(row.ID, phone, topic, city, status, row.CreatedAt), nil
}

// corrupt reports a stored row that breaks the entity's rules. It keeps the rule's message but
// not its kind: bad data in our database is our bug (500), not the client's input (422).
func corrupt(id uuid.UUID, field string, err error) error {
	return fmt.Errorf("stored lead %s has an invalid %s: %s", id, field, err.Error())
}

// mapError translates storage facts into the ports' vocabulary: no rows is "not found",
// contention and timeouts are postgres.ErrBusy, the rest is wrapped with what failed.
func mapError(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.ErrLeadNotFound
	}
	return fmt.Errorf("%s: %w", what, postgres.MapError(err))
}
