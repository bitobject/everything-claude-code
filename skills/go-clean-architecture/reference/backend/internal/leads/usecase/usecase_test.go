package usecase_test

import (
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/google/go-cmp/cmp"

	"example.com/app/internal/kernel/apperr"
	"example.com/app/internal/leads/contract"
	"example.com/app/internal/leads/entity"
	"example.com/app/internal/leads/usecase"
	"example.com/app/internal/leads/usecase/usecasetest"
)

var leadID = uuid.MustParse("019a7a3e-0000-7000-8000-000000000001")

func TestStoreContract(t *testing.T) {
	usecasetest.RunStoreContract(t, func(*testing.T) usecasetest.StoreUnderTest { return usecasetest.NewStore() })
}

// newCreateLead returns the use case at a fixed time; its first lead gets leadID.
func newCreateLead(store *usecasetest.Store, at time.Time) *usecase.CreateLead {
	first := true
	return usecase.NewCreateLead(store, usecasetest.FixedClock{T: at}, func() uuid.UUID {
		if first {
			first = false
			return leadID
		}
		return uuid.NewV7()
	})
}

func input(key string) usecase.CreateLeadInput {
	return usecase.CreateLeadInput{IdempotencyKey: key, Phone: "+79991234567", Topic: " Divorce ", City: "Kazan"}
}

func TestCreateLeadStoresAndPublishes(t *testing.T) {
	store := usecasetest.NewStore()
	got, err := newCreateLead(store, usecasetest.Now).Handle(t.Context(), input("key-0123456789abcdef"))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	want := usecase.LeadView{ID: leadID, Topic: "Divorce", City: "Kazan", Status: "new", CreatedAt: usecasetest.Now}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Handle() mismatch (-want +got):\n%s", diff)
	}
	if _, err := store.Leads().Get(t.Context(), leadID); err != nil {
		t.Errorf("lead was not stored: %v", err)
	}
	if diff := cmp.Diff([]usecase.Event{contract.LeadCreated{LeadID: leadID}}, store.Events()); diff != "" {
		t.Errorf("published events mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateLeadRepeatedKeyReturnsTheFirstLead(t *testing.T) {
	store := usecasetest.NewStore()
	uc := newCreateLead(store, usecasetest.Now)
	first, err := uc.Handle(t.Context(), input("key-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := uc.Handle(t.Context(), input("key-0123456789abcdef"))
	if err != nil {
		t.Fatalf("repeated Handle() error = %v", err)
	}
	if again != first {
		t.Errorf("repeated Handle() = %+v, want the first lead %+v", again, first)
	}
	if n := len(store.Events()); n != 1 {
		t.Errorf("published %d events, want 1", n)
	}
}

func TestCreateLeadKeyReusedWithAnotherBody(t *testing.T) {
	store := usecasetest.NewStore()
	uc := newCreateLead(store, usecasetest.Now)
	if _, err := uc.Handle(t.Context(), input("key-0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	other := input("key-0123456789abcdef")
	other.City = "Moscow"
	if _, err := uc.Handle(t.Context(), other); !errors.Is(err, usecase.ErrIdempotencyKeyReused) {
		t.Fatalf("Handle() error = %v, want %v", err, usecase.ErrIdempotencyKeyReused)
	}
}

func TestCreateLeadLimitPerPhone(t *testing.T) {
	store := usecasetest.NewStore()
	for i := range usecase.MaxLeadsPerPhone {
		uc := usecase.NewCreateLead(store, usecasetest.FixedClock{T: usecasetest.Now.Add(time.Duration(i) * time.Minute)}, uuid.NewV7)
		in := input("key-limit-000000000" + string(rune('a'+i)))
		in.Topic = "Divorce " + string(rune('a'+i)) // a new request, not a repeat
		if _, err := uc.Handle(t.Context(), in); err != nil {
			t.Fatalf("lead %d: %v", i, err)
		}
	}
	uc := usecase.NewCreateLead(store, usecasetest.FixedClock{T: usecasetest.Now.Add(time.Hour)}, uuid.NewV7)
	_, err := uc.Handle(t.Context(), input("key-limit-0000000000z"))
	if !errors.Is(err, usecase.ErrTooManyLeads) {
		t.Fatalf("Handle() error = %v, want %v", err, usecase.ErrTooManyLeads)
	}
	if apperr.KindOf(err) != apperr.KindRateLimited {
		t.Errorf("KindOf() = %v, want rate_limited", apperr.KindOf(err))
	}
}

func TestCreateLeadRejectsInvalidInputWithoutStoring(t *testing.T) {
	store := usecasetest.NewStore()
	in := input("key-0123456789abcdef")
	in.Phone = "8 (999) 123-45-67"
	_, err := newCreateLead(store, usecasetest.Now).Handle(t.Context(), in)
	if !errors.Is(err, entity.ErrInvalidPhone) {
		t.Fatalf("Handle() error = %v, want %v", err, entity.ErrInvalidPhone)
	}
	if apperr.KindOf(err) != apperr.KindInvalid {
		t.Errorf("KindOf() = %v, want invalid", apperr.KindOf(err))
	}
	if _, err := store.Leads().Get(t.Context(), leadID); !errors.Is(err, usecase.ErrLeadNotFound) {
		t.Errorf("a lead was stored: Get() = %v", err)
	}
}

func TestCreateLeadWrapsStorageErrors(t *testing.T) {
	store := usecasetest.NewStore()
	boom := errors.New("connection reset")
	store.Fail(boom)
	_, err := newCreateLead(store, usecasetest.Now).Handle(t.Context(), input("key-0123456789abcdef"))
	if !errors.Is(err, boom) {
		t.Fatalf("Handle() error = %v, want it to wrap %v", err, boom)
	}
}

func TestAssignLead(t *testing.T) {
	store := usecasetest.NewStore()
	if _, err := newCreateLead(store, usecasetest.Now).Handle(t.Context(), input("key-0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	uc := usecase.NewAssignLead(store.Leads())

	got, err := uc.Handle(t.Context(), leadID)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got.Status != "assigned" {
		t.Errorf("Status = %q, want assigned", got.Status)
	}
	stored, err := store.Leads().Get(t.Context(), leadID)
	if err != nil || stored.Status() != entity.StatusAssigned {
		t.Errorf("stored status = %v (%v), want assigned", stored.Status(), err)
	}

	_, err = uc.Handle(t.Context(), leadID)
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("second Handle() kind = %v, want conflict (err %v)", apperr.KindOf(err), err)
	}
}

func TestGetLeadNotFound(t *testing.T) {
	_, err := usecase.NewGetLead(usecasetest.NewStore().Queries()).Handle(t.Context(), leadID)
	if !errors.Is(err, usecase.ErrLeadNotFound) {
		t.Fatalf("Handle() error = %v, want %v", err, usecase.ErrLeadNotFound)
	}
}
