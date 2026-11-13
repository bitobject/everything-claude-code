package httpapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"uuid"

	"example.com/app/internal/api"
	"example.com/app/internal/api/apitest"
	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/leads/delivery/httpapi"
	"example.com/app/internal/leads/usecase"
	"example.com/app/internal/leads/usecase/usecasetest"
	"example.com/app/internal/platform/httpserver"
)

// newServer wires the real use cases on the in-memory store: the adapter is tested with what it
// really calls, and only storage is fake.
func newServer(store *usecasetest.Store) openapi.StrictServerInterface {
	return httpapi.New(httpapi.UseCases{
		Create: usecase.NewCreateLead(store, usecasetest.FixedClock{T: usecasetest.Now}, uuid.NewV7),
		Get:    usecase.NewGetLead(store.Queries()),
		Assign: usecase.NewAssignLead(store.Leads()),
	})
}

const key = `"0b8f2a6c-1d4e-4f7a-9c3b-5e6d7f8a9b0c"`

func createRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/leads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	return req
}

func withSession(req *http.Request) *http.Request {
	req.AddCookie(&http.Cookie{Name: api.SessionCookie, Value: "s"})
	return req
}

func problemOf(t *testing.T, rec *httptest.ResponseRecorder) httpserver.Problem {
	t.Helper()
	var p httpserver.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a problem: %v: %s", err, rec.Body)
	}
	return p
}

func TestCreateLead(t *testing.T) {
	store := usecasetest.NewStore()
	rec := apitest.Serve(t, newServer(store), createRequest(t, `{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	var got openapi.Lead
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if loc := rec.Header().Get("Location"); loc != "/v1/leads/"+got.Id.String() {
		t.Errorf("Location = %q, want /v1/leads/%s", loc, got.Id)
	}
	if got.Status != openapi.LeadStatusNew || got.City != "Kazan" {
		t.Errorf("body = %+v", got)
	}
	if strings.Contains(rec.Body.String(), "9991234567") {
		t.Error("the response contains the phone number")
	}
}

func TestCreateLeadRepeatedKey(t *testing.T) {
	server := newServer(usecasetest.NewStore())
	body := `{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`
	first := apitest.Serve(t, server, createRequest(t, body))
	again := apitest.Serve(t, server, createRequest(t, body))
	if again.Code != http.StatusCreated || again.Body.String() != first.Body.String() {
		t.Errorf("repeat = %d %s, want 201 with the first body %s", again.Code, again.Body, first.Body)
	}

	other := apitest.Serve(t, server, createRequest(t, `{"phone":"+79991234567","topic":"Divorce","city":"Moscow"}`))
	if other.Code != http.StatusUnprocessableEntity || problemOf(t, other).Code != "idempotency.key_reused" {
		t.Errorf("same key, other body = %d %s, want 422 idempotency.key_reused", other.Code, other.Body)
	}
}

func TestCreateLeadRateLimited(t *testing.T) {
	server := newServer(usecasetest.NewStore())
	var rec *httptest.ResponseRecorder
	for i := range usecase.MaxLeadsPerPhone + 1 {
		req := createRequest(t, `{"phone":"+79991234567","topic":"Divorce `+strconv.Itoa(i)+`","city":"Kazan"}`)
		req.Header.Set("Idempotency-Key", `"key-number-000000000`+strconv.Itoa(i)+`"`)
		rec = apitest.Serve(t, server, req)
	}
	if rec.Code != http.StatusTooManyRequests || problemOf(t, rec).Code != "lead.rate_limited" {
		t.Fatalf("over the limit = %d %s, want 429 lead.rate_limited", rec.Code, rec.Body)
	}
	if rec.Header().Get("Retry-After") != "3600" {
		t.Errorf("Retry-After = %q, want 3600", rec.Header().Get("Retry-After"))
	}
}

// TestInvalidRequestsNeverEchoInput: a 400 names the field and the rule, never the value.
func TestInvalidRequestsNeverEchoInput(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantErrors []httpserver.FieldError
	}{
		{
			name:       "phone breaks the pattern",
			body:       `{"phone":"+7999123456X","topic":"Divorce","city":"Kazan"}`,
			wantErrors: []httpserver.FieldError{{Pointer: "#/phone", Code: "pattern"}},
		},
		{
			name:       "unknown property",
			body:       `{"phone":"+79991234567","topic":"Divorce","city":"Kazan","name":"Secret Name"}`,
			wantErrors: []httpserver.FieldError{{Pointer: "#", Code: "additionalProperties"}},
		},
		{
			name: "missing and too short, both reported",
			body: `{"phone":"+79991234567","topic":"Di"}`,
			wantErrors: []httpserver.FieldError{
				{Pointer: "#/city", Code: "required"},
				{Pointer: "#/topic", Code: "minLength"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := apitest.Serve(t, newServer(usecasetest.NewStore()), createRequest(t, tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			p := problemOf(t, rec)
			if p.Code != "request.invalid" || len(p.Errors) != len(tt.wantErrors) {
				t.Fatalf("problem = %+v, want request.invalid with %v", p, tt.wantErrors)
			}
			for _, want := range tt.wantErrors {
				found := false
				for _, got := range p.Errors {
					found = found || got == want
				}
				if !found {
					t.Errorf("errors = %v, missing %v", p.Errors, want)
				}
			}
			for _, secret := range []string{"999123456", "Secret Name", `"Di"`, "pattern '", "Schema", "Value"} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Errorf("the response echoes %q: %s", secret, rec.Body)
				}
			}
		})
	}
}

func TestSecuredOperationsNeedTheSessionCookie(t *testing.T) {
	store := usecasetest.NewStore()
	server := newServer(store)
	created := apitest.Serve(t, server, createRequest(t, `{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`))
	var lead openapi.Lead
	if err := json.Unmarshal(created.Body.Bytes(), &lead); err != nil {
		t.Fatal(err)
	}
	path := "/v1/leads/" + lead.Id.String()

	anonymous := apitest.Serve(t, server, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	if anonymous.Code != http.StatusUnauthorized || problemOf(t, anonymous).Code != "auth.required" {
		t.Errorf("without the cookie = %d %s, want 401 auth.required", anonymous.Code, anonymous.Body)
	}
	got := apitest.Serve(t, server, withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)))
	if got.Code != http.StatusOK {
		t.Errorf("with the cookie = %d %s, want 200", got.Code, got.Body)
	}
}

func TestAssignLead(t *testing.T) {
	server := newServer(usecasetest.NewStore())
	created := apitest.Serve(t, server, createRequest(t, `{"phone":"+79991234567","topic":"Divorce","city":"Kazan"}`))
	var lead openapi.Lead
	if err := json.Unmarshal(created.Body.Bytes(), &lead); err != nil {
		t.Fatal(err)
	}
	assign := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/leads/"+lead.Id.String()+"/assign", nil)
		return apitest.Serve(t, server, withSession(req))
	}
	if rec := assign(); rec.Code != http.StatusOK {
		t.Fatalf("assign = %d %s, want 200", rec.Code, rec.Body)
	}
	if rec := assign(); rec.Code != http.StatusConflict || problemOf(t, rec).Code != "lead.not_assignable" {
		t.Errorf("second assign = %d %s, want 409 lead.not_assignable", rec.Code, rec.Body)
	}
}

func TestErrorsBecomeProblems(t *testing.T) {
	tests := []struct {
		name       string
		fail       error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", wantStatus: http.StatusNotFound, wantCode: "lead.not_found"},
		{name: "internal is hidden", fail: errors.New("connection reset by peer"), wantStatus: http.StatusInternalServerError, wantCode: "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := usecasetest.NewStore()
			store.Fail(tt.fail)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/leads/"+uuid.NewV7().String(), nil)
			rec := apitest.Serve(t, newServer(store), withSession(req))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if p := problemOf(t, rec); p.Code != tt.wantCode || p.Type != httpserver.ProblemTypeBase+tt.wantCode {
				t.Errorf("problem = %+v, want code and type for %q", p, tt.wantCode)
			}
			if strings.Contains(rec.Body.String(), "connection reset") {
				t.Error("internal error text leaked to the client")
			}
		})
	}
}
