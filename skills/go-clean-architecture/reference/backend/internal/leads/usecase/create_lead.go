package usecase

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
	"uuid"

	"example.com/app/internal/kernel/apperr"
	"example.com/app/internal/leads/contract"
	"example.com/app/internal/leads/entity"
)

// Errors of CreateLead.
var (
	ErrIdempotencyKeyReused = apperr.New(apperr.KindInvalid, "idempotency.key_reused",
		"this Idempotency-Key was already used with a different request")
	ErrTooManyLeads = apperr.New(apperr.KindRateLimited, "lead.rate_limited",
		"too many requests from this phone number, retry later").WithRetryAfter(time.Hour)
	// errNoIdempotencyKey is a caller's bug (the contract requires the header), so it is internal.
	errNoIdempotencyKey = errors.New("create lead: no idempotency key")
)

// Rules of CreateLead.
const (
	// IdempotencyKeyTTL is how long a key makes a repeated request return the first one's lead.
	IdempotencyKeyTTL = 24 * time.Hour
	// MaxLeadsPerPhone leads per phone number within LeadsPerPhoneWindow, against spam. Concurrent
	// requests can each see the count below the limit, so it is approximate; an exact limit would
	// lock per phone number.
	MaxLeadsPerPhone    = 3
	LeadsPerPhoneWindow = 24 * time.Hour
)

// CreateLead captures a lead from a public form.
type CreateLead struct {
	uow   UnitOfWork
	clock Clock
	newID func() uuid.UUID
}

// NewCreateLead wires the use case. newID is injected so tests get stable IDs; production
// passes uuid.NewV7.
func NewCreateLead(uow UnitOfWork, clock Clock, newID func() uuid.UUID) *CreateLead {
	return &CreateLead{uow: uow, clock: clock, newID: newID}
}

// CreateLeadInput is what a client submits.
type CreateLeadInput struct {
	IdempotencyKey string // without the quotes of the header's Structured Field form
	Phone          string
	Topic          string
	City           string
}

// Handle validates the input, then in one transaction: claims the idempotency key (a repeated
// request gets the lead the first one created), applies the per-phone limit, stores the lead and
// publishes LeadCreated.
func (uc *CreateLead) Handle(ctx context.Context, in CreateLeadInput) (LeadView, error) {
	if in.IdempotencyKey == "" {
		return LeadView{}, errNoIdempotencyKey
	}
	phone, err := entity.ParsePhone(in.Phone)
	if err != nil {
		return LeadView{}, err
	}
	topic, err := entity.ParseTopic(in.Topic)
	if err != nil {
		return LeadView{}, err
	}
	city, err := entity.ParseCity(in.City)
	if err != nil {
		return LeadView{}, err
	}
	now := uc.clock.Now()
	lead, err := entity.NewLead(uc.newID(), phone, topic, city, now)
	if err != nil {
		return LeadView{}, err
	}
	key := IdempotencyRecord{
		Key:         in.IdempotencyKey,
		Fingerprint: fingerprint(phone, topic, city),
		LeadID:      lead.ID(),
		CreatedAt:   now,
	}

	var out LeadView
	err = uc.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		prior, claimed, err := tx.IdempotencyKeys().Claim(ctx, key, now.Add(-IdempotencyKeyTTL))
		if err != nil {
			return err
		}
		if !claimed {
			if prior.Fingerprint != key.Fingerprint {
				return ErrIdempotencyKeyReused
			}
			first, err := tx.Leads().Get(ctx, prior.LeadID)
			if err != nil {
				return fmt.Errorf("lead %s of the idempotency key: %w", prior.LeadID, err)
			}
			out = viewOf(first)
			return nil
		}
		n, err := tx.Leads().CountCreatedSince(ctx, phone, now.Add(-LeadsPerPhoneWindow))
		if err != nil {
			return err
		}
		if n >= MaxLeadsPerPhone {
			return ErrTooManyLeads
		}
		if err := tx.Leads().Create(ctx, lead); err != nil {
			return err
		}
		if err := tx.Events().Publish(ctx, contract.LeadCreated{LeadID: lead.ID()}); err != nil {
			return err
		}
		out = viewOf(lead)
		return nil
	})
	if err != nil {
		return LeadView{}, fmt.Errorf("create lead: %w", err)
	}
	return out, nil
}

// fingerprint identifies the request's content, so a reused key with another body is caught.
func fingerprint(phone entity.Phone, topic entity.Topic, city entity.City) [sha256.Size]byte {
	return sha256.Sum256([]byte(phone.E164() + "\x00" + topic.String() + "\x00" + city.String()))
}
