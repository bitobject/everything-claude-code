package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/app/internal/api"
	"example.com/app/internal/api/apitest"
	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/platform/httpserver"
	"example.com/app/internal/platform/telemetry/telemetrytest"
)

// unreachable fails the test if a request gets past the validator.
type unreachable struct{ t *testing.T }

func (u unreachable) CreateLead(context.Context, openapi.CreateLeadRequestObject) (openapi.CreateLeadResponseObject, error) {
	u.t.Error("CreateLead was called")
	return nil, nil
}

func (u unreachable) GetLead(context.Context, openapi.GetLeadRequestObject) (openapi.GetLeadResponseObject, error) {
	u.t.Error("GetLead was called")
	return nil, nil
}

func (u unreachable) AssignLead(context.Context, openapi.AssignLeadRequestObject) (openapi.AssignLeadResponseObject, error) {
	u.t.Error("AssignLead was called")
	return nil, nil
}

const validKey = `"0b8f2a6c-1d4e-4f7a-9c3b-5e6d7f8a9b0c"`

func TestRejectedRequestsAreProblems(t *testing.T) {
	tests := []struct {
		name, method, path, contentType, key, body string
		session                                    bool
		wantStatus                                 int
		wantCode                                   string
		wantErrors                                 []httpserver.FieldError
		wantAllow                                  string
	}{
		{name: "unknown route", method: http.MethodGet, path: "/v1/nope", wantStatus: 404, wantCode: "request.not_found"},
		{name: "wrong method", method: http.MethodDelete, path: "/v1/leads", wantStatus: 405, wantCode: "request.method_not_allowed", wantAllow: "POST"},
		{
			name: "body too large", method: http.MethodPost, path: "/v1/leads", contentType: "application/json", key: validKey,
			body: `{"phone":"` + strings.Repeat("1", 2<<20) + `"}`, wantStatus: 413, wantCode: "request.too_large",
		},
		{
			name: "wrong media type", method: http.MethodPost, path: "/v1/leads", contentType: "text/plain", key: validKey,
			body: "phone=+79991234567", wantStatus: 415, wantCode: "request.unsupported_media_type",
		},
		{
			name: "malformed JSON", method: http.MethodPost, path: "/v1/leads", contentType: "application/json", key: validKey,
			body: `{"phone":`, wantStatus: 400, wantCode: "request.invalid",
			wantErrors: []httpserver.FieldError{{Pointer: "#", Code: "malformed"}},
		},
		{
			name: "missing Idempotency-Key", method: http.MethodPost, path: "/v1/leads", contentType: "application/json",
			body: `{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`, wantStatus: 400, wantCode: "request.invalid",
			wantErrors: []httpserver.FieldError{{Parameter: "Idempotency-Key", Code: "required"}},
		},
		{
			name: "unquoted Idempotency-Key", method: http.MethodPost, path: "/v1/leads", contentType: "application/json", key: "0b8f2a6c1d4e4f7a",
			body: `{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`, wantStatus: 400, wantCode: "request.invalid",
			wantErrors: []httpserver.FieldError{{Parameter: "Idempotency-Key", Code: "pattern"}},
		},
		{name: "no session", method: http.MethodGet, path: "/v1/leads/019a7a3e-0000-7000-8000-000000000001", wantStatus: 401, wantCode: "auth.required"},
		{
			name: "path parameter of the wrong format", method: http.MethodGet, path: "/v1/leads/not-a-uuid", session: true,
			wantStatus: 400, wantCode: "request.invalid", wantErrors: []httpserver.FieldError{{Parameter: "leadId", Code: "format"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.key != "" {
				req.Header.Set("Idempotency-Key", tt.key)
			}
			if tt.session {
				req.AddCookie(&http.Cookie{Name: "__Host-session", Value: "s"})
			}
			rec := apitest.Serve(t, unreachable{t}, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q", ct)
			}
			var p httpserver.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("not a problem: %v", err)
			}
			if p.Code != tt.wantCode || p.Status != tt.wantStatus || p.Type != httpserver.ProblemTypeBase+tt.wantCode {
				t.Errorf("problem = %+v, want status %d code %q", p, tt.wantStatus, tt.wantCode)
			}
			if len(p.Errors) != len(tt.wantErrors) {
				t.Fatalf("errors = %v, want %v", p.Errors, tt.wantErrors)
			}
			for i := range p.Errors {
				if p.Errors[i] != tt.wantErrors[i] {
					t.Errorf("errors[%d] = %v, want %v", i, p.Errors[i], tt.wantErrors[i])
				}
			}
			if tt.wantAllow != "" && rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", rec.Header().Get("Allow"), tt.wantAllow)
			}
		})
	}
}

// Spans and request metrics are named by the operation's template whichever layer answers: the
// validator (400, 401) or the generated binding inside the mux. A request for no operation is
// named by its method alone, so scanners can't add series.
func TestSpansAreNamedByOperation(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		session                  bool
		want                     string
	}{
		{name: "no such operation", method: http.MethodGet, path: "/v1/nope/019a7a3e-0000-7000-8000-000000000001", want: "GET"},
		{name: "rejected by the validator", method: http.MethodPost, path: "/v1/leads", body: `{"phone":`, want: "POST /v1/leads"},
		{name: "no session", method: http.MethodGet, path: "/v1/leads/019a7a3e-0000-7000-8000-000000000001", want: "GET /v1/leads/{leadId}"},
		{name: "rejected by the binding", method: http.MethodGet, path: "/v1/leads/not-a-uuid", session: true, want: "GET /v1/leads/{leadId}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := telemetrytest.Install(t)
			v1, err := api.New(unreachable{t}, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			h := httpserver.Observe(httpserver.Chain(v1, httpserver.RequestID), "api:8080", httpserver.ClientIP{})

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", validKey)
			if tt.session {
				req.AddCookie(&http.Cookie{Name: api.SessionCookie, Value: "s"})
			}
			h.ServeHTTP(httptest.NewRecorder(), req)

			if spans := rec.Spans(); len(spans) != 1 || spans[0].Name() != tt.want {
				t.Errorf("spans = %v, want one named %q", telemetrytest.Names(spans), tt.want)
			}
		})
	}
}
