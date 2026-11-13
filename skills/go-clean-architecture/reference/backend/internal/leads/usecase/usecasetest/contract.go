package usecasetest

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
	"uuid"

	"example.com/app/internal/leads/entity"
	"example.com/app/internal/leads/usecase"
)

// StoreUnderTest is one implementation of the module's storage ports.
type StoreUnderTest interface {
	usecase.UnitOfWork
	usecase.IdempotencyKeyPurger
	Leads() usecase.LeadRepository
	Queries() usecase.LeadQueries
}

// RunStoreContract checks the behavior the use cases rely on. newStore returns an empty store.
func RunStoreContract(t *testing.T, newStore func(t *testing.T) StoreUnderTest) {
	t.Run("create then get returns the same lead", func(t *testing.T) {
		s := newStore(t)
		lead := NewLead(t, "+79991234567", Now)
		if err := s.Leads().Create(t.Context(), lead); err != nil {
			t.Fatalf("Create() = %v", err)
		}
		got, err := s.Leads().Get(t.Context(), lead.ID())
		if err != nil {
			t.Fatalf("Get() = %v", err)
		}
		SameLead(t, got, lead)
	})

	t.Run("creating a lead with a taken ID fails", func(t *testing.T) {
		s := newStore(t)
		lead := NewLead(t, "+79991234567", Now)
		if err := s.Leads().Create(t.Context(), lead); err != nil {
			t.Fatal(err)
		}
		if err := s.Leads().Create(t.Context(), lead); err == nil {
			t.Error("second Create() with the same ID = nil, want an error")
		}
	})

	t.Run("get and update of a missing lead are not found", func(t *testing.T) {
		s := newStore(t)
		id := uuid.NewV7()
		if _, err := s.Leads().Get(t.Context(), id); !errors.Is(err, usecase.ErrLeadNotFound) {
			t.Errorf("Get() = %v, want %v", err, usecase.ErrLeadNotFound)
		}
		_, err := s.Leads().Update(t.Context(), id, func(*entity.Lead) error { return nil })
		if !errors.Is(err, usecase.ErrLeadNotFound) {
			t.Errorf("Update() = %v, want %v", err, usecase.ErrLeadNotFound)
		}
		if _, err := s.Queries().Lead(t.Context(), id); !errors.Is(err, usecase.ErrLeadNotFound) {
			t.Errorf("Queries().Lead() = %v, want %v", err, usecase.ErrLeadNotFound)
		}
	})

	t.Run("update saves the whole aggregate fn leaves", func(t *testing.T) {
		s := newStore(t)
		lead := NewLead(t, "+79991234567", Now)
		if err := s.Leads().Create(t.Context(), lead); err != nil {
			t.Fatal(err)
		}
		updated, err := s.Leads().Update(t.Context(), lead.ID(), func(l *entity.Lead) error { return l.Assign() })
		if err != nil {
			t.Fatalf("Update() = %v", err)
		}
		got, err := s.Leads().Get(t.Context(), lead.ID())
		if err != nil {
			t.Fatal(err)
		}
		SameLead(t, got, updated)
		if got.Status() != entity.StatusAssigned {
			t.Errorf("stored status = %q, want %q", got.Status(), entity.StatusAssigned)
		}
	})

	t.Run("update that fails leaves the lead unchanged", func(t *testing.T) {
		s := newStore(t)
		lead := NewLead(t, "+79991234567", Now)
		if err := s.Leads().Create(t.Context(), lead); err != nil {
			t.Fatal(err)
		}
		boom := errors.New("boom")
		_, err := s.Leads().Update(t.Context(), lead.ID(), func(l *entity.Lead) error {
			if err := l.Assign(); err != nil {
				return err
			}
			return boom // after a change: the change must not be saved
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Update() = %v, want %v", err, boom)
		}
		got, err := s.Leads().Get(t.Context(), lead.ID())
		if err != nil {
			t.Fatal(err)
		}
		SameLead(t, got, lead)
	})

	t.Run("a unit of work that fails stores nothing", func(t *testing.T) {
		s := newStore(t)
		lead := NewLead(t, "+79991234567", Now)
		boom := errors.New("boom")
		err := s.Do(t.Context(), func(ctx context.Context, tx usecase.Tx) error {
			if err := tx.Leads().Create(ctx, lead); err != nil {
				return err
			}
			if _, _, err := tx.IdempotencyKeys().Claim(ctx, record("key-of-a-failed-request", lead.ID(), Now), Now.Add(-time.Hour)); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Do() = %v, want %v", err, boom)
		}
		if _, err := s.Leads().Get(t.Context(), lead.ID()); !errors.Is(err, usecase.ErrLeadNotFound) {
			t.Errorf("Get() after rollback = %v, want %v", err, usecase.ErrLeadNotFound)
		}
		claimed := claim(t, s, record("key-of-a-failed-request", uuid.NewV7(), Now), Now.Add(-time.Hour))
		if !claimed {
			t.Error("the key of a rolled-back request is still taken")
		}
	})

	t.Run("count by phone since a time", func(t *testing.T) {
		s := newStore(t)
		for _, l := range []*entity.Lead{
			NewLead(t, "+79991234567", Now.Add(-2*time.Hour)),
			NewLead(t, "+79991234567", Now.Add(-30*time.Minute)),
			NewLead(t, "+79990000000", Now.Add(-30*time.Minute)),
		} {
			if err := s.Leads().Create(t.Context(), l); err != nil {
				t.Fatal(err)
			}
		}
		phone, _ := entity.ParsePhone("+79991234567")
		n, err := s.Leads().CountCreatedSince(t.Context(), phone, Now.Add(-time.Hour))
		if err != nil || n != 1 {
			t.Errorf("CountCreatedSince() = %d, %v; want 1, nil", n, err)
		}
	})

	t.Run("claim: first wins, a live key returns the first record, an expired key is taken over", func(t *testing.T) {
		s := newStore(t)
		first := record("6f1c0e5a8d0b4a49", uuid.NewV7(), Now.Add(-2*time.Hour))
		if !claim(t, s, first, Now.Add(-24*time.Hour)) {
			t.Fatal("first Claim() not claimed")
		}
		var prior usecase.IdempotencyRecord
		var claimed bool
		err := s.Do(t.Context(), func(ctx context.Context, tx usecase.Tx) error {
			var err error
			prior, claimed, err = tx.IdempotencyKeys().Claim(ctx, record(first.Key, uuid.NewV7(), Now), Now.Add(-24*time.Hour))
			return err
		})
		if err != nil || claimed {
			t.Fatalf("second Claim() = claimed %v, %v; want false, nil", claimed, err)
		}
		if prior.LeadID != first.LeadID || prior.Fingerprint != first.Fingerprint {
			t.Errorf("prior = %+v, want the first record %+v", prior, first)
		}
		if !claim(t, s, record(first.Key, uuid.NewV7(), Now), Now.Add(-time.Hour)) {
			t.Error("Claim() of an expired key not claimed")
		}
	})

	t.Run("purge deletes the keys created before the time, and only those", func(t *testing.T) {
		s := newStore(t)
		cutoff := Now.Add(-24 * time.Hour)
		old := record("key-created-before-the-cutoff", uuid.NewV7(), cutoff.Add(-time.Microsecond))
		atCutoff := record("key-created-at-the-cutoff", uuid.NewV7(), cutoff)
		for _, rec := range []usecase.IdempotencyRecord{old, atCutoff} {
			if !claim(t, s, rec, Now.Add(-48*time.Hour)) {
				t.Fatalf("Claim(%s) not claimed", rec.Key)
			}
		}
		n, err := s.PurgeIdempotencyKeys(t.Context(), cutoff)
		if err != nil || n != 1 {
			t.Fatalf("PurgeIdempotencyKeys() = %d, %v; want 1, nil", n, err)
		}
		// With expiredBefore long ago, only a key that is gone can be claimed again.
		if !claim(t, s, record(old.Key, uuid.NewV7(), Now), Now.Add(-48*time.Hour)) {
			t.Error("the purged key is still taken")
		}
		if claim(t, s, record(atCutoff.Key, uuid.NewV7(), Now), Now.Add(-48*time.Hour)) {
			t.Error("the key created at the cutoff was purged; Claim still treats it as live")
		}
	})

	t.Run("the read side returns the view", func(t *testing.T) {
		s := newStore(t)
		lead := NewLead(t, "+79991234567", Now)
		if err := s.Leads().Create(t.Context(), lead); err != nil {
			t.Fatal(err)
		}
		v, err := s.Queries().Lead(t.Context(), lead.ID())
		if err != nil {
			t.Fatal(err)
		}
		want := usecase.LeadView{ID: lead.ID(), Topic: "Divorce", City: "Kazan", Status: "new", CreatedAt: Now}
		if v != want {
			t.Errorf("Lead() = %+v, want %+v", v, want)
		}
	})
}

// Now is the time the suite and the use-case tests use, in UTC, at microsecond precision
// (PostgreSQL's), so values survive a round trip unchanged.
var Now = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

// NewLead returns a valid new lead with topic Divorce in Kazan.
func NewLead(t testing.TB, phone string, createdAt time.Time) *entity.Lead {
	t.Helper()
	p, err := entity.ParsePhone(phone)
	if err != nil {
		t.Fatal(err)
	}
	topic, _ := entity.ParseTopic("Divorce")
	city, _ := entity.ParseCity("Kazan")
	l, err := entity.NewLead(uuid.NewV7(), p, topic, city, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// SameLead fails t unless got and want agree on every field.
func SameLead(t testing.TB, got, want *entity.Lead) {
	t.Helper()
	if got.ID() != want.ID() || got.Phone() != want.Phone() || got.Topic() != want.Topic() ||
		got.City() != want.City() || got.Status() != want.Status() || !got.CreatedAt().Equal(want.CreatedAt()) {
		t.Errorf("lead = {%v %v %v %v %v %v}, want {%v %v %v %v %v %v}",
			got.ID(), got.Phone(), got.Topic(), got.City(), got.Status(), got.CreatedAt(),
			want.ID(), want.Phone(), want.Topic(), want.City(), want.Status(), want.CreatedAt())
	}
}

func record(key string, leadID uuid.UUID, at time.Time) usecase.IdempotencyRecord {
	return usecase.IdempotencyRecord{Key: key, Fingerprint: sha256.Sum256([]byte(key)), LeadID: leadID, CreatedAt: at}
}

func claim(t *testing.T, s StoreUnderTest, rec usecase.IdempotencyRecord, expiredBefore time.Time) bool {
	t.Helper()
	var claimed bool
	err := s.Do(t.Context(), func(ctx context.Context, tx usecase.Tx) error {
		var err error
		_, claimed, err = tx.IdempotencyKeys().Claim(ctx, rec, expiredBefore)
		return err
	})
	if err != nil {
		t.Fatalf("Claim() = %v", err)
	}
	return claimed
}
