package entity_test

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
	"uuid"

	"example.com/app/internal/leads/entity"
)

func TestParsePhone(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{name: "valid", in: "+79991234567"},
		{name: "missing plus", in: "79991234567", wantErr: entity.ErrInvalidPhone},
		{name: "too short", in: "+7999123456", wantErr: entity.ErrInvalidPhone},
		{name: "letter", in: "+7999123456a", wantErr: entity.ErrInvalidPhone},
		{name: "other country", in: "+19991234567", wantErr: entity.ErrInvalidPhone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := entity.ParsePhone(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParsePhone(%q) error = %v, want %v", tt.in, err, tt.wantErr)
			}
			if err == nil && p.E164() != tt.in {
				t.Errorf("ParsePhone(%q).E164() = %q, want %q", tt.in, p.E164(), tt.in)
			}
		})
	}
}

func TestPhoneIsMaskedWhenPrinted(t *testing.T) {
	p, err := entity.ParsePhone("+79991234567")
	if err != nil {
		t.Fatal(err)
	}
	var logged strings.Builder
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("lead", "value", p) // the key is irrelevant; Phone.LogValue masks
	for _, out := range []string{fmt.Sprint(p), fmt.Sprintf("%+v", p), logged.String()} {
		if strings.Contains(out, "9991234567") {
			t.Errorf("printed phone %q contains the number", out)
		}
	}
}

func TestParseTopicAndCityTrim(t *testing.T) {
	topic, err := entity.ParseTopic("  Divorce  ")
	if err != nil || topic.String() != "Divorce" {
		t.Errorf("ParseTopic = %q, %v; want Divorce, nil", topic, err)
	}
	if _, err := entity.ParseTopic("  ab  "); !errors.Is(err, entity.ErrInvalidTopic) {
		t.Errorf("ParseTopic(short after trim) error = %v, want %v", err, entity.ErrInvalidTopic)
	}
	if _, err := entity.ParseCity(strings.Repeat("я", 101)); !errors.Is(err, entity.ErrInvalidCity) {
		t.Errorf("ParseCity(101 runes) error = %v, want %v", err, entity.ErrInvalidCity)
	}
}

func TestNewLeadRefusesZeroValues(t *testing.T) {
	phone, topic, city := validParts(t)
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		id      uuid.UUID
		phone   entity.Phone
		topic   entity.Topic
		city    entity.City
		wantErr error
	}{
		{name: "zero id", phone: phone, topic: topic, city: city, wantErr: entity.ErrInvalidID},
		{name: "zero phone", id: uuid.NewV7(), topic: topic, city: city, wantErr: entity.ErrInvalidPhone},
		{name: "zero topic", id: uuid.NewV7(), phone: phone, city: city, wantErr: entity.ErrInvalidTopic},
		{name: "zero city", id: uuid.NewV7(), phone: phone, topic: topic, wantErr: entity.ErrInvalidCity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := entity.NewLead(tt.id, tt.phone, tt.topic, tt.city, now); !errors.Is(err, tt.wantErr) {
				t.Errorf("NewLead() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestLeadAssign(t *testing.T) {
	phone, topic, city := validParts(t)
	lead, err := entity.NewLead(uuid.NewV7(), phone, topic, city, time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if err := lead.Assign(); err != nil {
		t.Fatalf("first Assign() = %v, want nil", err)
	}
	if got := lead.Status(); got != entity.StatusAssigned {
		t.Errorf("Status() = %q, want %q", got, entity.StatusAssigned)
	}
	if err := lead.Assign(); !errors.Is(err, entity.ErrNotAssignable) {
		t.Errorf("second Assign() = %v, want %v", err, entity.ErrNotAssignable)
	}
}

func validParts(t *testing.T) (entity.Phone, entity.Topic, entity.City) {
	t.Helper()
	phone, err := entity.ParsePhone("+79991234567")
	if err != nil {
		t.Fatal(err)
	}
	topic, err := entity.ParseTopic("Divorce and property")
	if err != nil {
		t.Fatal(err)
	}
	city, err := entity.ParseCity("Kazan")
	if err != nil {
		t.Fatal(err)
	}
	return phone, topic, city
}

func FuzzParsePhone(f *testing.F) {
	f.Add("+79991234567")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		p, err := entity.ParsePhone(s)
		if err == nil && p.E164() != s {
			t.Errorf("valid phone %q round-trips as %q", s, p.E164())
		}
	})
}
