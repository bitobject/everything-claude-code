// Package apitest runs requests through the production API stack (api.New) and checks every
// response against the contract: the schema of its status and media type, and, for a problem,
// that its code is one the operation declares in x-problem-codes.
package apitest

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"example.com/app/internal/api"
	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/platform/httpserver"
)

// Serve runs req through the API as cmd/api mounts it (body limit, then api.New(si)) and
// returns the checked response.
func Serve(t *testing.T, si openapi.StrictServerInterface, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	h, err := api.New(si, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	httpserver.LimitBody(h).ServeHTTP(rec, req)
	CheckContract(t, req, rec)
	return rec
}

// CheckContract fails t when the response of a contract operation breaks the contract.
// Requests the contract has no operation for (404, 405) aren't checked.
func CheckContract(t *testing.T, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	spec, err := openapi.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.Servers = nil
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		t.Fatal(err)
	}
	route, params, err := router.FindRoute(req)
	if err != nil {
		return
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: req, PathParams: params, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		},
		Status: rec.Code,
		Header: rec.Header(),
		Body:   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	if err := openapi3filter.ValidateResponse(t.Context(), in); err != nil {
		t.Errorf("response violates the contract: %v\nbody: %s", err, rec.Body)
	}
	if rec.Code < http.StatusBadRequest {
		return
	}
	var p struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Errorf("%d response is not a problem document: %s", rec.Code, rec.Body)
		return
	}
	declared, _ := route.Operation.Extensions["x-problem-codes"].(map[string]any)
	codes, _ := declared[strconv.Itoa(rec.Code)].([]any)
	if !slices.Contains(codes, any(p.Code)) {
		t.Errorf("%s returned %d %q, which its x-problem-codes don't list (%v)", route.Operation.OperationID, rec.Code, p.Code, codes)
	}
}
