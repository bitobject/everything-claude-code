// Package events delivers a module's events to the modules that subscribe to them. A publisher
// doesn't know its subscribers: cmd registers them at startup (Subscribe), and Publish turns
// one event into one River job per subscriber, inserted in the publisher's transaction. The
// jobs exist only if the state change commits (transactional outbox), and each subscriber
// retries on its own. River runs one worker per job kind, which is why each subscriber gets a
// job of its own kind instead of all of them sharing the event's.
//
// An event published while nobody subscribes is dropped; a subscriber added later doesn't
// receive past events.
package events

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// Event is a fact a module publishes: a struct in that module's contract package, named in the
// past tense, with a versioned name ("leads.lead_created.v1"). Fields are only ever added.
type Event interface{ EventName() string }

// Bus is the subscription table. Build it in cmd, register every subscriber, then share it; it
// is not safe to Subscribe while publishing.
type Bus struct {
	river *river.Client[pgx.Tx]
	subs  map[string][]func(Event) river.JobArgs
}

// NewBus returns a bus that inserts jobs with client. The API process passes an insert-only
// client (no queues, never started); the worker passes its own.
func NewBus(client *river.Client[pgx.Tx]) *Bus {
	return &Bus{river: client, subs: map[string][]func(Event) river.JobArgs{}}
}

// Subscribe registers a subscriber to events of type E: job turns the event into the
// subscriber's job args (with the IDs it needs, never personal data).
func Subscribe[E Event](b *Bus, job func(E) river.JobArgs) {
	var zero E
	name := zero.EventName()
	b.subs[name] = append(b.subs[name], func(e Event) river.JobArgs {
		ev, _ := e.(E) // Publish looks subscribers up by name, so e is an E
		return job(ev)
	})
}

// Publish inserts one job per subscriber of e in tx.
func (b *Bus) Publish(ctx context.Context, tx pgx.Tx, e Event) error {
	subs := b.subs[e.EventName()]
	if len(subs) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, 0, len(subs))
	for _, job := range subs {
		params = append(params, river.InsertManyParams{Args: job(e)})
	}
	if _, err := b.river.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("publish %s: %w", e.EventName(), err)
	}
	return nil
}
