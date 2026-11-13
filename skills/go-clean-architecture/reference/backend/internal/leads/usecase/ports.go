// Package usecase holds the leads module's application rules: one type per use case
// (CreateLead, AssignLead, GetLead, PurgeIdempotencyKeys), the ports they need, and plain
// input/output structs. It
// imports only the standard library, the shared kernel and this module's entity and contract
// packages: never pgx, net/http, generated API types or another module.
package usecase

import (
	"context"
	"crypto/sha256"
	"time"
	"uuid"

	"example.com/app/internal/leads/contract"
	"example.com/app/internal/leads/entity"
)

// ErrLeadNotFound is returned when no lead has the ID. It is the contract's sentinel, so other
// modules match the same value.
var ErrLeadNotFound = contract.ErrLeadNotFound

// LeadRepository loads and saves the Lead aggregate.
type LeadRepository interface {
	Create(ctx context.Context, lead *entity.Lead) error
	// Get returns the lead with id, or ErrLeadNotFound.
	Get(ctx context.Context, id uuid.UUID) (*entity.Lead, error)
	// Update locks the lead, runs fn and saves the whole aggregate fn leaves, atomically. fn's
	// error leaves the stored lead unchanged and is returned as is. fn does no I/O.
	Update(ctx context.Context, id uuid.UUID, fn func(*entity.Lead) error) (*entity.Lead, error)
	// CountCreatedSince counts the leads from phone created at or after since.
	CountCreatedSince(ctx context.Context, phone entity.Phone, since time.Time) (int, error)
}

// LeadQueries reads leads for display: straight from storage into views, with no aggregate in
// between (CQRS-lite: writes go through entities, reads don't need their rules).
type LeadQueries interface {
	// Lead returns the view of the lead with id, or ErrLeadNotFound.
	Lead(ctx context.Context, id uuid.UUID) (LeadView, error)
}

// IdempotencyRecord is what a key remembers: a fingerprint of the request and the lead it made.
type IdempotencyRecord struct {
	Key         string
	Fingerprint [sha256.Size]byte
	LeadID      uuid.UUID
	CreatedAt   time.Time
}

// IdempotencyKeys remembers which lead each Idempotency-Key created.
type IdempotencyKeys interface {
	// Claim stores rec, unless rec.Key already belongs to a request made at or after
	// expiredBefore: then it stores nothing and returns that request's record, claimed false.
	// An older record is replaced. Concurrent claims of one key wait for each other.
	Claim(ctx context.Context, rec IdempotencyRecord, expiredBefore time.Time) (prior IdempotencyRecord, claimed bool, err error)
}

// IdempotencyKeyPurger deletes Idempotency-Key records that no request can match any more.
type IdempotencyKeyPurger interface {
	// PurgeIdempotencyKeys deletes the records created before createdBefore and returns how many.
	PurgeIdempotencyKeys(ctx context.Context, createdBefore time.Time) (int64, error)
}

// Event is a fact this module publishes: a type from the contract package.
type Event interface{ EventName() string }

// Publisher publishes events in the transaction of the change they report: subscribers see
// an event only if the change commits.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

// Tx gives a unit of work this module's repositories, bound to one transaction. Only this
// module's: never call another module, an external service or anything slow inside Do, because
// the transaction holds row locks and a pool connection until fn returns. To make another
// module react, publish an event.
type Tx interface {
	Leads() LeadRepository
	IdempotencyKeys() IdempotencyKeys
	Events() Publisher
}

// UnitOfWork runs fn in one transaction: everything fn did through tx commits when fn returns
// nil and rolls back when it returns an error, which Do returns as is.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Clock is the time port; everything it returns is UTC.
type Clock interface {
	Now() time.Time
}
