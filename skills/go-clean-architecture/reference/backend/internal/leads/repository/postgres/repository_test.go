package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"example.com/app/internal/leads/contract"
	"example.com/app/internal/leads/entity"
	"example.com/app/internal/leads/repository/postgres"
	"example.com/app/internal/leads/usecase"
	"example.com/app/internal/leads/usecase/usecasetest"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/postgres/pgtest"
	"example.com/app/migrations"
)

// TestStoreContract runs the suite the fake runs, on a real PostgreSQL.
func TestStoreContract(t *testing.T) {
	usecasetest.RunStoreContract(t, func(t *testing.T) usecasetest.StoreUnderTest {
		return postgres.NewStore(pgtest.New(t, migrations.FS), events.NewBus(nil))
	})
}

var errRollback = errors.New("roll back")

type probeArgs struct {
	LeadID string `json:"lead_id"`
}

func (probeArgs) Kind() string { return "test.lead_created" }

// TestPublishInsertsJobsInTheTransaction checks the outbox: a subscriber's job exists only if
// the unit of work commits.
func TestPublishInsertsJobsInTheTransaction(t *testing.T) {
	pool := pgtest.New(t, migrations.FS)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "river"})
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus(client)
	events.Subscribe(bus, func(e contract.LeadCreated) river.JobArgs { return probeArgs{LeadID: e.LeadID.String()} })
	store := postgres.NewStore(pool, bus)

	committed := usecasetest.NewLead(t, "+79991234567", usecasetest.Now)
	if err := store.Do(t.Context(), func(ctx context.Context, tx usecase.Tx) error {
		if err := tx.Leads().Create(ctx, committed); err != nil {
			return err
		}
		return tx.Events().Publish(ctx, contract.LeadCreated{LeadID: committed.ID()})
	}); err != nil {
		t.Fatal(err)
	}
	rolledBack := usecasetest.NewLead(t, "+79991234567", usecasetest.Now)
	_ = store.Do(t.Context(), func(ctx context.Context, tx usecase.Tx) error {
		if err := tx.Events().Publish(ctx, contract.LeadCreated{LeadID: rolledBack.ID()}); err != nil {
			return err
		}
		return errRollback
	})

	rows, err := pool.Query(t.Context(), `SELECT args->>'lead_id' FROM river.river_job WHERE kind = 'test.lead_created'`)
	if err != nil {
		t.Fatal(err)
	}
	leadIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(leadIDs) != 1 || leadIDs[0] != committed.ID().String() {
		t.Errorf("jobs for leads %v, want only %s", leadIDs, committed.ID())
	}
}

// TestConcurrentAssignHasOneWinner: Update locks the row (FOR NO KEY UPDATE), so concurrent
// assigns queue; the first wins and the others see an assigned lead.
func TestConcurrentAssignHasOneWinner(t *testing.T) {
	leads := postgres.NewStore(pgtest.New(t, migrations.FS), events.NewBus(nil)).Leads()
	l := usecasetest.NewLead(t, "+79991234567", usecasetest.Now)
	if err := leads.Create(t.Context(), l); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = leads.Update(t.Context(), l.ID(), func(l *entity.Lead) error { return l.Assign() })
		})
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, entity.ErrNotAssignable):
			t.Errorf("Update() unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Errorf("successful assigns = %d, want 1", won)
	}
}
