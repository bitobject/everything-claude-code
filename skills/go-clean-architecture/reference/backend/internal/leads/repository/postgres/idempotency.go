package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"example.com/app/internal/leads/repository/postgres/sqlcgen"
	"example.com/app/internal/leads/usecase"
)

// KeyPurger deletes expired Idempotency-Key records, outside any unit of work.
type KeyPurger struct{ q *sqlcgen.Queries }

var _ usecase.IdempotencyKeyPurger = KeyPurger{}

// NewKeyPurger returns the purger on pool.
func NewKeyPurger(pool *pgxpool.Pool) KeyPurger { return KeyPurger{q: sqlcgen.New(pool)} }

// PurgeIdempotencyKeys implements usecase.IdempotencyKeyPurger.
func (p KeyPurger) PurgeIdempotencyKeys(ctx context.Context, createdBefore time.Time) (int64, error) {
	n, err := p.q.DeleteIdempotencyKeysCreatedBefore(ctx, createdBefore)
	if err != nil {
		return 0, mapError(err, "purge idempotency keys")
	}
	return n, nil
}

// idempotencyKeys stores Idempotency-Key records; it exists only inside a unit of work, so the
// claim commits or rolls back with the lead it made.
type idempotencyKeys struct{ q *sqlcgen.Queries }

func (k idempotencyKeys) Claim(ctx context.Context, rec usecase.IdempotencyRecord, expiredBefore time.Time) (usecase.IdempotencyRecord, bool, error) {
	n, err := k.q.ClaimIdempotencyKey(ctx, sqlcgen.ClaimIdempotencyKeyParams{
		Key:           rec.Key,
		Fingerprint:   rec.Fingerprint[:],
		LeadID:        rec.LeadID,
		CreatedAt:     rec.CreatedAt,
		ExpiredBefore: expiredBefore,
	})
	if err != nil {
		return usecase.IdempotencyRecord{}, false, mapError(err, "claim idempotency key")
	}
	if n == 1 {
		return usecase.IdempotencyRecord{}, true, nil
	}
	row, err := k.q.GetIdempotencyKey(ctx, rec.Key)
	if err != nil {
		return usecase.IdempotencyRecord{}, false, mapError(err, "read idempotency key")
	}
	prior := usecase.IdempotencyRecord{Key: row.Key, LeadID: row.LeadID, CreatedAt: row.CreatedAt}
	if copy(prior.Fingerprint[:], row.Fingerprint) != sha256.Size {
		return usecase.IdempotencyRecord{}, false, fmt.Errorf("stored idempotency key has a %d-byte fingerprint", len(row.Fingerprint))
	}
	return prior, false, nil
}
