// Package entity holds the leads module's enterprise rules: the Lead aggregate, its value
// objects and invariants. It imports only the standard library and the shared kernel.
package entity

import (
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"example.com/app/internal/kernel/apperr"
)

// Errors returned by the leads entity. Kinds tell adapters how to report them.
var (
	ErrInvalidPhone  = apperr.New(apperr.KindInvalid, "lead.invalid_phone", "phone must be +7 followed by 10 digits")
	ErrInvalidTopic  = apperr.New(apperr.KindInvalid, "lead.invalid_topic", "topic must be 3 to 200 characters")
	ErrInvalidCity   = apperr.New(apperr.KindInvalid, "lead.invalid_city", "city must be 2 to 100 characters")
	ErrInvalidID     = apperr.New(apperr.KindInternal, "lead.invalid_id", "a lead needs an ID")
	ErrNotAssignable = apperr.New(apperr.KindConflict, "lead.not_assignable", "only a new lead can be assigned")
	ErrUnknownStatus = apperr.New(apperr.KindInternal, "lead.unknown_status", "unknown lead status")
)

// Phone is a Russian mobile number in E.164 form. It is personal data: fmt and slog print it
// masked; E164 returns the whole number for storage and delivery. The zero value is not valid.
type Phone struct{ e164 string }

// ParsePhone validates s. It does not normalize: the contract already fixes the format.
func ParsePhone(s string) (Phone, error) {
	if len(s) != 12 || !strings.HasPrefix(s, "+7") {
		return Phone{}, ErrInvalidPhone
	}
	for _, r := range s[2:] {
		if r < '0' || r > '9' {
			return Phone{}, ErrInvalidPhone
		}
	}
	return Phone{e164: s}, nil
}

// E164 returns the full number, "+79991234567".
func (p Phone) E164() string { return p.e164 }

// String returns the masked number, "+7******4567", so the full number never lands in a log or
// an error message by accident.
func (p Phone) String() string {
	if p.e164 == "" {
		return ""
	}
	return p.e164[:2] + "******" + p.e164[len(p.e164)-4:]
}

// LogValue makes slog print the masked number.
func (p Phone) LogValue() slog.Value { return slog.StringValue(p.String()) }

// Topic is the legal question in the client's words, 3 to 200 characters after trimming.
type Topic struct{ s string }

// ParseTopic trims and validates s.
func ParseTopic(s string) (Topic, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 3 || n > 200 {
		return Topic{}, ErrInvalidTopic
	}
	return Topic{s: s}, nil
}

func (t Topic) String() string { return t.s }

// City is where the client needs help, 2 to 100 characters after trimming.
type City struct{ s string }

// ParseCity trims and validates s.
func ParseCity(s string) (City, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 2 || n > 100 {
		return City{}, ErrInvalidCity
	}
	return City{s: s}, nil
}

func (c City) String() string { return c.s }

// Status is where a lead is in its lifecycle.
type Status string

// Lead statuses.
const (
	StatusNew      Status = "new"
	StatusAssigned Status = "assigned"
	StatusClosed   Status = "closed"
)

// ParseStatus is used by adapters that read a stored status.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusNew, StatusAssigned, StatusClosed:
		return st, nil
	default:
		return "", ErrUnknownStatus
	}
}

// Lead is the aggregate root. Fields are unexported so every change goes through a method
// that keeps the invariants.
type Lead struct {
	id        uuid.UUID
	phone     Phone
	topic     Topic
	city      City
	status    Status
	createdAt time.Time
}

// NewLead creates a lead in status new. The caller supplies the ID and the time, so the
// entity stays deterministic and testable. The value objects come from their Parse functions;
// a zero one is refused.
func NewLead(id uuid.UUID, phone Phone, topic Topic, city City, now time.Time) (*Lead, error) {
	switch {
	case id == uuid.Nil():
		return nil, ErrInvalidID
	case phone == Phone{}:
		return nil, ErrInvalidPhone
	case topic == Topic{}:
		return nil, ErrInvalidTopic
	case city == City{}:
		return nil, ErrInvalidCity
	}
	return &Lead{id: id, phone: phone, topic: topic, city: city, status: StatusNew, createdAt: now.UTC()}, nil
}

// RestoreLead rebuilds a lead from storage. Only repository adapters call it (internal/archtest
// checks); it does not re-run creation rules, because stored data was valid when it was written.
func RestoreLead(id uuid.UUID, phone Phone, topic Topic, city City, status Status, createdAt time.Time) *Lead {
	return &Lead{id: id, phone: phone, topic: topic, city: city, status: status, createdAt: createdAt.UTC()}
}

// Assign moves a new lead to assigned.
func (l *Lead) Assign() error {
	if l.status != StatusNew {
		return ErrNotAssignable
	}
	l.status = StatusAssigned
	return nil
}

// ID returns the lead's identifier.
func (l *Lead) ID() uuid.UUID { return l.id }

// Phone returns the client's phone. It is personal data: never log Phone().E164().
func (l *Lead) Phone() Phone { return l.phone }

// Topic returns the legal topic the client described.
func (l *Lead) Topic() Topic { return l.topic }

// City returns the city the client needs help in.
func (l *Lead) City() City { return l.city }

// Status returns the lifecycle status.
func (l *Lead) Status() Status { return l.status }

// CreatedAt returns when the lead was captured, in UTC.
func (l *Lead) CreatedAt() time.Time { return l.createdAt }
