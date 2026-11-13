package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"example.com/app/internal/leads/delivery/worker"
	"example.com/app/internal/leads/usecase"
	"example.com/app/internal/leads/usecase/usecasetest"
)

// TestPurgeArgsJSON pins the job's encoding: jobs already in the queue must still decode after
// a deploy. Change this test only by adding fields.
func TestPurgeArgsJSON(t *testing.T) {
	got, err := json.Marshal(worker.PurgeIdempotencyKeysArgs{})
	if err != nil || string(got) != `{}` {
		t.Errorf("encoding = %s, %v; want {}", got, err)
	}
}

func purgeJob() *river.Job[worker.PurgeIdempotencyKeysArgs] {
	return &river.Job[worker.PurgeIdempotencyKeysArgs]{JobRow: &rivertype.JobRow{}}
}

func newWorker(store *usecasetest.Store) *worker.PurgeIdempotencyKeys {
	clock := usecasetest.FixedClock{T: usecasetest.Now}
	return worker.NewPurgeIdempotencyKeys(usecase.NewPurgeIdempotencyKeys(store, clock))
}

func TestPurgeDeletesKeysOlderThanTheTTL(t *testing.T) {
	store := usecasetest.NewStore()
	claimAt := func(key string, at time.Time) {
		t.Helper()
		err := store.Do(t.Context(), func(ctx context.Context, tx usecase.Tx) error {
			_, _, err := tx.IdempotencyKeys().Claim(ctx, usecase.IdempotencyRecord{Key: key, LeadID: uuid.NewV7(), CreatedAt: at}, at)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	claimAt("key-older-than-the-ttl", usecasetest.Now.Add(-usecase.IdempotencyKeyTTL-time.Second))
	claimAt("key-younger-than-the-ttl", usecasetest.Now.Add(-time.Hour))

	if err := newWorker(store).Work(t.Context(), purgeJob()); err != nil {
		t.Fatalf("Work() = %v", err)
	}
	n, err := store.PurgeIdempotencyKeys(t.Context(), usecasetest.Now) // what is left
	if err != nil || n != 1 {
		t.Errorf("keys left after the purge = %d, %v; want 1 (the younger one)", n, err)
	}
}

func TestPurgeRetriesStorageErrors(t *testing.T) {
	store := usecasetest.NewStore()
	boom := errors.New("connection refused")
	store.Fail(boom)
	err := newWorker(store).Work(t.Context(), purgeJob())
	if _, cancelled := errors.AsType[*rivertype.JobCancelError](err); cancelled || !errors.Is(err, boom) {
		t.Errorf("Work() = %v, want %v returned for a retry", err, boom)
	}
}
