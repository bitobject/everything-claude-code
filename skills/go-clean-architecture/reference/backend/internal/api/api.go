// Package api assembles the HTTP API under /v1: the request validator (the contract decides
// what a valid request is), the generated routes and strict handlers, and the mapping of every
// error to a problem document. cmd/api and the delivery tests both build the API with New, so
// tests run the stack production runs.
//
// Order: the validator wraps the whole /v1 subtree, so unknown routes (404), wrong methods
// (405), oversized bodies (413), wrong media types (415), missing sessions (401) and bodies or
// parameters that break the contract (400) are all answered here, as problem documents, before
// a handler runs. Handlers then map only use-case errors (httpserver.ErrorWriter).
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"example.com/app/internal/gen/openapi"
	"example.com/app/internal/platform/httpserver"
)

// SessionCookie is the cookie of the contract's `session` security scheme.
const SessionCookie = "__Host-session"

var errNoSession = errors.New("no session cookie")

// New returns the /v1 handler serving si, the union of every module's handlers.
func New(si openapi.StrictServerInterface, log *slog.Logger) (http.Handler, error) {
	spec, err := openapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load embedded spec: %w", err)
	}
	var router routers.Router // for the Allow header of a 405; set below, before any request
	validate := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true, // match paths whatever host serves them
		Options: openapi3filter.Options{
			MultiError:         true, // every field error at once, for forms
			AuthenticationFunc: requireSessionCookie,
		},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
			writeRequestProblem(w, r, err, opts, router, log)
		},
	})
	router, err = gorillamux.NewRouter(spec) // the validator dropped spec.Servers: no host matching here either
	if err != nil {
		return nil, fmt.Errorf("route the spec: %w", err)
	}

	strict := openapi.NewStrictHandlerWithOptions(si, nil, openapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: httpserver.ErrorWriter(log),
	})
	routes := openapi.HandlerWithOptions(strict, openapi.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		ErrorHandlerFunc: badRequest,
	})
	return validate(routes), nil
}

// requireSessionCookie makes the contract's security declaration enforceable: an operation that
// doesn't say `security: []` can't be called without the session cookie. It checks only that the
// cookie is there. Looking the session up and deciding what its user may do is authentication
// (a middleware in front of the API) and authorization (the use case); neither is in this
// reference.
func requireSessionCookie(_ context.Context, in *openapi3filter.AuthenticationInput) error {
	if in.SecuritySchemeName != "session" {
		return fmt.Errorf("security scheme %q has no check", in.SecuritySchemeName)
	}
	if _, err := in.RequestValidationInput.Request.Cookie(SessionCookie); err != nil {
		return errNoSession
	}
	return nil
}

// writeRequestProblem answers a request the validator rejected. It never puts the validator's
// message in the response: in JSON Schema 2020-12 mode it quotes the submitted values (phone
// numbers) and dumps the schema. Field errors carry a pointer and the rule, nothing else.
func writeRequestProblem(w http.ResponseWriter, r *http.Request, err error, opts nethttpmiddleware.ErrorHandlerOpts, router routers.Router, log *slog.Logger) {
	if opts.MatchedRoute == nil {
		if errors.Is(err, routers.ErrMethodNotAllowed) {
			w.Header().Set("Allow", strings.Join(allowedMethods(router, r), ", "))
			httpserver.WriteProblem(w, r, httpserver.NewProblem(http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed"))
			return
		}
		httpserver.WriteProblem(w, r, httpserver.NewProblem(http.StatusNotFound, "request.not_found", "no such API operation"))
		return
	}
	v := classify(err, r, opts.MatchedRoute.Route)
	switch {
	case v.unknown != nil:
		log.ErrorContext(r.Context(), "request validation failed", "method", r.Method, "path", r.URL.Path, "err", v.unknown)
		httpserver.WriteProblem(w, r, httpserver.NewProblem(http.StatusInternalServerError, "internal", "internal error"))
	case v.unauthenticated:
		httpserver.WriteProblem(w, r, httpserver.NewProblem(http.StatusUnauthorized, "auth.required", "sign in to use this operation"))
	case v.tooLarge:
		httpserver.WriteProblem(w, r, httpserver.NewProblem(http.StatusRequestEntityTooLarge, "request.too_large", "the request body is too large"))
	case v.unsupported:
		httpserver.WriteProblem(w, r, httpserver.NewProblem(http.StatusUnsupportedMediaType, "request.unsupported_media_type", "the request body has an unsupported media type"))
	default:
		writeInvalid(w, r, v.fields)
	}
}

func writeInvalid(w http.ResponseWriter, r *http.Request, fields []httpserver.FieldError) {
	p := httpserver.NewProblem(http.StatusBadRequest, "request.invalid", "the request does not match the API contract")
	p.Errors = fields
	httpserver.WriteProblem(w, r, p)
}

type verdict struct {
	unauthenticated, tooLarge, unsupported bool
	fields                                 []httpserver.FieldError
	unknown                                error
}

func classify(err error, r *http.Request, route *routers.Route) verdict {
	var v verdict
	var walk func(error)
	walk = func(err error) {
		switch e := err.(type) { //nolint:errorlint // the validator's own types, never wrapped at this level
		case openapi3.MultiError:
			for _, x := range e {
				walk(x)
			}
		case *openapi3filter.SecurityRequirementsError:
			v.unauthenticated = true
		case *openapi3filter.RequestError:
			switch {
			case isMaxBytes(e.Err):
				v.tooLarge = true
			case e.RequestBody != nil && e.Err == nil && !mediaTypeAccepted(r, route):
				v.unsupported = true
			default:
				v.fields = append(v.fields, requestFieldErrors(e)...)
			}
		default:
			v.unknown = err
		}
	}
	walk(err)
	return v
}

func isMaxBytes(err error) bool {
	_, ok := errors.AsType[*http.MaxBytesError](err)
	return ok
}

// mediaTypeAccepted reports whether the operation's request body accepts r's Content-Type.
func mediaTypeAccepted(r *http.Request, route *routers.Route) bool {
	body := route.Operation.RequestBody
	if body == nil || body.Value == nil {
		return true
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && body.Value.Content.Get(mt) != nil
}

// requestFieldErrors turns one rejected parameter or body into field errors.
func requestFieldErrors(e *openapi3filter.RequestError) []httpserver.FieldError {
	param := ""
	if e.Parameter != nil {
		param = e.Parameter.Name
	}
	switch {
	case errors.Is(e.Err, openapi3filter.ErrInvalidRequired), errors.Is(e.Err, openapi3filter.ErrInvalidEmptyValue):
		return []httpserver.FieldError{at(param, nil, "required")}
	case isParseError(e.Err):
		return []httpserver.FieldError{at(param, nil, "malformed")}
	}
	if se, ok := errors.AsType[*openapi3.SchemaError](e.Err); ok {
		var out []httpserver.FieldError
		for _, leaf := range schemaLeaves(se) {
			out = append(out, leafErrors(param, leaf)...)
		}
		return out
	}
	return []httpserver.FieldError{at(param, nil, "invalid")}
}

func isParseError(err error) bool {
	_, ok := errors.AsType[*openapi3filter.ParseError](err)
	return ok
}

// schemaLeaves returns the individual failures inside a SchemaError: with several failures the
// 2020-12 validator nests them in Origin.
func schemaLeaves(se *openapi3.SchemaError) []*openapi3.SchemaError {
	if inner, ok := errors.AsType[openapi3.MultiError](se.Origin); ok {
		var out []*openapi3.SchemaError
		for _, x := range inner {
			if leaf, ok := errors.AsType[*openapi3.SchemaError](x); ok {
				out = append(out, schemaLeaves(leaf)...)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []*openapi3.SchemaError{se}
}

// leafErrors maps one schema failure. For "required" the validator points at the object, so the
// missing properties are named from our own schema (never from the request). Unknown properties
// ("additionalProperties") point at their object without naming them: the names are the
// client's input.
func leafErrors(param string, se *openapi3.SchemaError) []httpserver.FieldError {
	rule := se.SchemaField
	if rule == "" {
		rule = "invalid"
	}
	path := se.JSONPointer()
	if rule == "required" && se.Schema != nil {
		present, _ := se.Value.(map[string]any)
		var out []httpserver.FieldError
		for _, name := range se.Schema.Required {
			if _, ok := present[name]; !ok {
				out = append(out, at(param, append(slices.Clone(path), name), "required"))
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []httpserver.FieldError{at(param, path, rule)}
}

// at builds a field error: a parameter by name, or a body location as a JSON Pointer fragment.
func at(param string, path []string, rule string) httpserver.FieldError {
	if param != "" {
		return httpserver.FieldError{Parameter: param, Code: rule}
	}
	var b strings.Builder
	b.WriteString("#")
	for _, p := range path {
		b.WriteString("/")
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(p))
	}
	return httpserver.FieldError{Pointer: b.String(), Code: rule}
}

// badRequest answers what the generated code couldn't bind after validation passed: a path
// parameter whose format the validator doesn't check (format: uuid is an annotation in JSON
// Schema 2020-12), or a body the strict handler couldn't decode.
func badRequest(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpserver.FieldError
	switch e := err.(type) { //nolint:errorlint // the generated code returns these types unwrapped
	case *openapi.InvalidParamFormatError:
		fe = httpserver.FieldError{Parameter: e.ParamName, Code: "format"}
	case *openapi.RequiredParamError:
		fe = httpserver.FieldError{Parameter: e.ParamName, Code: "required"}
	case *openapi.RequiredHeaderError:
		fe = httpserver.FieldError{Parameter: e.ParamName, Code: "required"}
	case *openapi.TooManyValuesForParamError:
		fe = httpserver.FieldError{Parameter: e.ParamName, Code: "multiple"}
	case *openapi.UnmarshalingParamError:
		fe = httpserver.FieldError{Parameter: e.ParamName, Code: "malformed"}
	case *openapi.UnescapedCookieParamError:
		fe = httpserver.FieldError{Parameter: e.ParamName, Code: "malformed"}
	default:
		fe = httpserver.FieldError{Pointer: "#", Code: "malformed"}
	}
	writeInvalid(w, r, []httpserver.FieldError{fe})
}

// allowedMethods lists the methods the contract has for r's path, for the Allow header of a 405.
func allowedMethods(router routers.Router, r *http.Request) []string {
	var allow []string
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		c := r.Clone(r.Context())
		c.Method = m
		if _, _, err := router.FindRoute(c); err == nil {
			allow = append(allow, m)
		}
	}
	return allow
}
