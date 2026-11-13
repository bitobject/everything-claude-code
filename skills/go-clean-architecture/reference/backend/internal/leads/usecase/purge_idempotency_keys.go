package usecase

import (
	"context"
	"fmt"
)

// PurgeIdempotencyKeys deletes the Idempotency-Key records older than IdempotencyKeyTTL: Claim
// treats them as expired already. A record keeps a fingerprint of the request, which is derived
// from the phone number (personal data: a hash of a phone number is reversed by trying every
// number), so it is kept only while a retry can still match it. delivery/worker runs it hourly.
type PurgeIdempotencyKeys struct {
	keys  IdempotencyKeyPurger
	clock Clock
}

// NewPurgeIdempotencyKeys wires the use case.
func NewPurgeIdempotencyKeys(keys IdempotencyKeyPurger, clock Clock) *PurgeIdempotencyKeys {
	return &PurgeIdempotencyKeys{keys: keys, clock: clock}
}

// Handle deletes the expired records and returns how many it deleted. Running it twice is
// harmless.
func (uc *PurgeIdempotencyKeys) Handle(ctx context.Context) (int64, error) {
	n, err := uc.keys.PurgeIdempotencyKeys(ctx, uc.clock.Now().Add(-IdempotencyKeyTTL))
	if err != nil {
		return 0, fmt.Errorf("purge idempotency keys: %w", err)
	}
	return n, nil
}
