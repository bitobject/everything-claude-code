// Package facade is the in-process entry to the leads module for other modules: it implements
// leads/contract.Reader with the module's use cases. cmd builds it and hands it to the gateways
// and workers of other modules, which know only the contract package.
package facade

import (
	"context"
	"uuid"

	"example.com/app/internal/leads/contract"
	"example.com/app/internal/leads/usecase"
)

// Facade implements the leads module's public API.
type Facade struct{ get *usecase.GetLead }

var _ contract.Reader = (*Facade)(nil)

// New returns the facade over the module's use cases.
func New(get *usecase.GetLead) *Facade { return &Facade{get: get} }

// Lead returns what other modules may know about the lead, or contract.ErrLeadNotFound.
func (f *Facade) Lead(ctx context.Context, id uuid.UUID) (contract.Lead, error) {
	v, err := f.get.Handle(ctx, id)
	if err != nil {
		return contract.Lead{}, err
	}
	return contract.Lead{ID: v.ID, Topic: v.Topic, City: v.City, Status: v.Status, CreatedAt: v.CreatedAt}, nil
}
