package usecase

import (
	"time"
	"uuid"

	"example.com/app/internal/leads/entity"
)

// LeadView is what callers may see of a lead: everything but the phone number, which is
// personal data. Every use case returns it; the HTTP adapter and the facade map it.
type LeadView struct {
	ID        uuid.UUID
	Topic     string
	City      string
	Status    string
	CreatedAt time.Time
}

func viewOf(l *entity.Lead) LeadView {
	return LeadView{
		ID:        l.ID(),
		Topic:     l.Topic().String(),
		City:      l.City().String(),
		Status:    string(l.Status()),
		CreatedAt: l.CreatedAt(),
	}
}
