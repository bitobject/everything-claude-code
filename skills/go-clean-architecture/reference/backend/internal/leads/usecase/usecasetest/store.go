// Package usecasetest provides Store, an in-memory implementation of the leads module's
// storage ports for use-case and handler tests, and RunStoreContract, the behavior every
// implementation must have. The fake and repository/postgres run the same suite, so the fake
// can't quietly drift from PostgreSQL.
package usecasetest

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"
	"uuid"

	"example.com/app/internal/leads/entity"
	"example.com/app/internal/leads/usecase"
)

// state is everything the fake stores. Leads are stored as values: a caller's pointer never
// aliases what is stored, so a change survives only if the port saves it.
type state struct {
	leads  map[uuid.UUID]entity.Lead
	keys   map[string]usecase.IdempotencyRecord
	events []usecase.Event
}

func (s *state) clone() *state {
	c := &state{
		leads:  make(map[uuid.UUID]entity.Lead, len(s.leads)),
		keys:   make(map[string]usecase.IdempotencyRecord, len(s.keys)),
		events: append([]usecase.Event(nil), s.events...),
	}
	maps.Copy(c.leads, s.leads)
	maps.Copy(c.keys, s.keys)
	return c
}

// Store is the fake. Transactions run one at a time (Do holds the lock), on a copy of the state
// that replaces it only when fn succeeds.
type Store struct {
	mu  sync.Mutex
	st  *state
	err error
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{st: &state{leads: map[uuid.UUID]entity.Lead{}, keys: map[string]usecase.IdempotencyRecord{}}}
}

var (
	_ usecase.UnitOfWork           = (*Store)(nil)
	_ usecase.IdempotencyKeyPurger = (*Store)(nil)
)

// Fail makes every call return err (nil restores normal behavior), like a database that is down.
func (s *Store) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// Do implements usecase.UnitOfWork.
func (s *Store) Do(ctx context.Context, fn func(context.Context, usecase.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	work := s.st.clone()
	if err := fn(ctx, txView{st: work}); err != nil {
		return err
	}
	s.st = work
	return nil
}

// PurgeIdempotencyKeys implements usecase.IdempotencyKeyPurger.
func (s *Store) PurgeIdempotencyKeys(_ context.Context, createdBefore time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	var n int64
	for key, rec := range s.st.keys {
		if rec.CreatedAt.Before(createdBefore) {
			delete(s.st.keys, key)
			n++
		}
	}
	return n, nil
}

// Leads returns the repository outside a unit of work.
func (s *Store) Leads() usecase.LeadRepository { return poolLeads{s: s} }

// Queries returns the read side.
func (s *Store) Queries() usecase.LeadQueries { return queries{s: s} }

// Events returns the events of committed transactions, in order.
func (s *Store) Events() []usecase.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]usecase.Event(nil), s.st.events...)
}

// txView is the Tx a unit of work gets: it works on the transaction's copy, without locking
// (Do holds the lock).
type txView struct{ st *state }

func (t txView) Leads() usecase.LeadRepository            { return txLeads(t) }
func (t txView) IdempotencyKeys() usecase.IdempotencyKeys { return keys(t) }
func (t txView) Events() usecase.Publisher                { return publisher(t) }

type txLeads struct{ st *state }

func (r txLeads) Create(_ context.Context, l *entity.Lead) error {
	if _, ok := r.st.leads[l.ID()]; ok {
		return fmt.Errorf("lead %s already exists", l.ID()) // a primary key violation in PostgreSQL
	}
	r.st.leads[l.ID()] = *l
	return nil
}

func (r txLeads) Get(_ context.Context, id uuid.UUID) (*entity.Lead, error) {
	l, ok := r.st.leads[id]
	if !ok {
		return nil, usecase.ErrLeadNotFound
	}
	return &l, nil
}

func (r txLeads) Update(_ context.Context, id uuid.UUID, fn func(*entity.Lead) error) (*entity.Lead, error) {
	l, ok := r.st.leads[id]
	if !ok {
		return nil, usecase.ErrLeadNotFound
	}
	if err := fn(&l); err != nil {
		return nil, err
	}
	r.st.leads[id] = l
	out := l
	return &out, nil
}

func (r txLeads) CountCreatedSince(_ context.Context, phone entity.Phone, since time.Time) (int, error) {
	n := 0
	for _, l := range r.st.leads {
		if l.Phone() == phone && !l.CreatedAt().Before(since) {
			n++
		}
	}
	return n, nil
}

type keys struct{ st *state }

func (k keys) Claim(_ context.Context, rec usecase.IdempotencyRecord, expiredBefore time.Time) (usecase.IdempotencyRecord, bool, error) {
	if prior, ok := k.st.keys[rec.Key]; ok && !prior.CreatedAt.Before(expiredBefore) {
		return prior, false, nil
	}
	k.st.keys[rec.Key] = rec
	return usecase.IdempotencyRecord{}, true, nil
}

type publisher struct{ st *state }

func (p publisher) Publish(_ context.Context, e usecase.Event) error {
	p.st.events = append(p.st.events, e)
	return nil
}

// poolLeads is the repository outside a unit of work: each call is its own transaction.
type poolLeads struct{ s *Store }

func (r poolLeads) Create(ctx context.Context, l *entity.Lead) error {
	return r.s.Do(ctx, func(ctx context.Context, tx usecase.Tx) error { return tx.Leads().Create(ctx, l) })
}

func (r poolLeads) Get(ctx context.Context, id uuid.UUID) (*entity.Lead, error) {
	var l *entity.Lead
	err := r.s.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
		var err error
		l, err = tx.Leads().Get(ctx, id)
		return err
	})
	return l, err
}

func (r poolLeads) Update(ctx context.Context, id uuid.UUID, fn func(*entity.Lead) error) (*entity.Lead, error) {
	var l *entity.Lead
	err := r.s.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
		var err error
		l, err = tx.Leads().Update(ctx, id, fn)
		return err
	})
	return l, err
}

func (r poolLeads) CountCreatedSince(ctx context.Context, phone entity.Phone, since time.Time) (int, error) {
	var n int
	err := r.s.Do(ctx, func(ctx context.Context, tx usecase.Tx) error {
		var err error
		n, err = tx.Leads().CountCreatedSince(ctx, phone, since)
		return err
	})
	return n, err
}

type queries struct{ s *Store }

func (q queries) Lead(ctx context.Context, id uuid.UUID) (usecase.LeadView, error) {
	l, err := poolLeads(q).Get(ctx, id)
	if err != nil {
		return usecase.LeadView{}, err
	}
	return usecase.LeadView{
		ID: l.ID(), Topic: l.Topic().String(), City: l.City().String(),
		Status: string(l.Status()), CreatedAt: l.CreatedAt(),
	}, nil
}

// FixedClock is a Clock that always returns T.
type FixedClock struct{ T time.Time }

// Now returns c.T.
func (c FixedClock) Now() time.Time { return c.T }
