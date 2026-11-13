// Package contract is the leads module's public API: the only package of this module that
// other modules import (from their gateways and workers). It holds what the module promises to
// the rest of the system: the facade other modules call, the data it returns, the events it
// publishes and the errors they can match. Change it the way you change the HTTP contract: add,
// never rename or remove; a breaking change to an event is a new event version published
// alongside the old one. It imports only the standard library and the shared kernel.
package contract

import (
	"context"
	"time"
	"uuid"

	"example.com/app/internal/kernel/apperr"
)

// ErrLeadNotFound is returned when no lead has the ID.
var ErrLeadNotFound = apperr.New(apperr.KindNotFound, "lead.not_found", "lead not found")

// Lead is what other modules may know about a lead. The phone number is personal data and
// stays inside the module.
type Lead struct {
	ID        uuid.UUID
	Topic     string
	City      string
	Status    string // "new", "assigned", "closed"; more may be added
	CreatedAt time.Time
}

// Reader is the facade other modules call to read leads. leads/delivery/facade implements it;
// cmd hands it to the modules that need it.
type Reader interface {
	Lead(ctx context.Context, id uuid.UUID) (Lead, error)
}

// LeadCreated is published in the transaction that stores a new lead. Subscribers receive it as
// a job of their own (platform/events), so it carries the ID only: they read what they need
// through Reader, and no personal data sits in the job queue.
type LeadCreated struct {
	LeadID uuid.UUID
}

// EventName names the event and its version.
func (LeadCreated) EventName() string { return "leads.lead_created.v1" }
