// Package httpapi is the leads module's HTTP adapter. It implements the leads operations of the
// generated openapi.StrictServerInterface: translate the request, call a use case, translate
// the result. Errors are returned as is; httpserver.ErrorWriter maps them to problems.
package httpapi

import (
	"context"
	"strings"

	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/leads/usecase"
)

// UseCases are the use cases the adapter calls: the real types, so handler tests run them with
// fake ports instead of faking the use cases.
type UseCases struct {
	Create *usecase.CreateLead
	Get    *usecase.GetLead
	Assign *usecase.AssignLead
}

// Handler serves the leads operations of the API.
type Handler struct{ uc UseCases }

// New returns a Handler that delegates to uc.
func New(uc UseCases) *Handler { return &Handler{uc: uc} }

// CreateLead implements POST /v1/leads.
func (h *Handler) CreateLead(ctx context.Context, req openapi.CreateLeadRequestObject) (openapi.CreateLeadResponseObject, error) {
	v, err := h.uc.Create.Handle(ctx, usecase.CreateLeadInput{
		IdempotencyKey: unquote(req.Params.IdempotencyKey),
		Phone:          req.Body.Phone,
		Topic:          req.Body.Topic,
		City:           req.Body.City,
	})
	if err != nil {
		return nil, err
	}
	return openapi.CreateLead201JSONResponse{
		Body:    toLead(v),
		Headers: openapi.CreateLead201ResponseHeaders{Location: "/v1/leads/" + v.ID.String()},
	}, nil
}

// GetLead implements GET /v1/leads/{leadId}.
func (h *Handler) GetLead(ctx context.Context, req openapi.GetLeadRequestObject) (openapi.GetLeadResponseObject, error) {
	v, err := h.uc.Get.Handle(ctx, req.LeadId)
	if err != nil {
		return nil, err
	}
	return openapi.GetLead200JSONResponse(toLead(v)), nil
}

// AssignLead implements POST /v1/leads/{leadId}/assign.
func (h *Handler) AssignLead(ctx context.Context, req openapi.AssignLeadRequestObject) (openapi.AssignLeadResponseObject, error) {
	v, err := h.uc.Assign.Handle(ctx, req.LeadId)
	if err != nil {
		return nil, err
	}
	return openapi.AssignLead200JSONResponse(toLead(v)), nil
}

// unquote strips the quotes of a Structured Field String; the contract's pattern guarantees them.
func unquote(sf string) string {
	return strings.TrimSuffix(strings.TrimPrefix(sf, `"`), `"`)
}

func toLead(v usecase.LeadView) openapi.Lead {
	return openapi.Lead{
		Id:        v.ID,
		Topic:     v.Topic,
		City:      v.City,
		Status:    openapi.LeadStatus(v.Status),
		CreatedAt: v.CreatedAt,
	}
}
