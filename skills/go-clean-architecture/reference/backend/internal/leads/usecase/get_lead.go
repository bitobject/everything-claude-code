package usecase

import (
	"context"
	"fmt"
	"uuid"
)

// GetLead returns one lead for display. Reads go straight to a view through LeadQueries; the
// use case is where "who may see this lead" goes once requests carry a principal.
type GetLead struct {
	queries LeadQueries
}

// NewGetLead wires the use case.
func NewGetLead(queries LeadQueries) *GetLead { return &GetLead{queries: queries} }

// Handle returns the lead with id, or ErrLeadNotFound.
func (uc *GetLead) Handle(ctx context.Context, id uuid.UUID) (LeadView, error) {
	v, err := uc.queries.Lead(ctx, id)
	if err != nil {
		return LeadView{}, fmt.Errorf("get lead %s: %w", id, err)
	}
	return v, nil
}
