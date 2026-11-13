package usecase

import (
	"context"
	"fmt"
	"uuid"

	"example.com/app/internal/leads/entity"
)

// AssignLead moves a new lead to assigned. The state rule lives in the entity; the use case
// only orchestrates. One aggregate changes, so Update's own transaction is enough.
type AssignLead struct {
	leads LeadRepository
}

// NewAssignLead wires the use case.
func NewAssignLead(leads LeadRepository) *AssignLead { return &AssignLead{leads: leads} }

// Handle assigns the lead with id.
func (uc *AssignLead) Handle(ctx context.Context, id uuid.UUID) (LeadView, error) {
	lead, err := uc.leads.Update(ctx, id, func(l *entity.Lead) error { return l.Assign() })
	if err != nil {
		return LeadView{}, fmt.Errorf("assign lead %s: %w", id, err)
	}
	return viewOf(lead), nil
}
