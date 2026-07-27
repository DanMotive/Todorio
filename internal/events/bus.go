// Package events: in-memory event bus for SSE (subscriptions keyed by user id).
package events

import "sync"

type Event struct {
	Type string `json:"type"` // task.updated | comment.created | notification | pulse.changed | announcement
	Data any    `json:"data,omitempty"`
}

// Tap observes every event that passes through the bus, whether or not anyone is subscribed.
//
// The bus was built for SSE, so its only delivery path is "to these user ids, if they happen to
// be connected right now". Outgoing webhooks need the opposite: every event, regardless of who
// is watching. Adding one observer here is what keeps that requirement out of the handlers —
// the alternative was a dispatcher call in tasks.go, social.go and every other file that
// publishes, which is a lot of edits to files that have nothing to do with webhooks.
type Tap func(userIDs []int64, e Event)

type Bus struct {
	mu   sync.Mutex
	subs map[int64]map[chan Event]struct{}
	tap  Tap
}

func New() *Bus { return &Bus{subs: map[int64]map[chan Event]struct{}{}} }

// SetTap installs the observer. Intended to be called once during startup, before the server
// begins handling requests; a second call replaces the first.
func (b *Bus) SetTap(t Tap) {
	b.mu.Lock()
	b.tap = t
	b.mu.Unlock()
}

func (b *Bus) Subscribe(userID int64) (chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	if b.subs[userID] == nil {
		b.subs[userID] = map[chan Event]struct{}{}
	}
	b.subs[userID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs[userID], ch)
		b.mu.Unlock()
	}
}

// Publish — non-blocking delivery; slow subscribers miss the event.
func (b *Bus) Publish(userIDs []int64, e Event) {
	b.mu.Lock()
	tap := b.tap
	for _, id := range userIDs {
		for ch := range b.subs[id] {
			select {
			case ch <- e:
			default:
			}
		}
	}
	b.mu.Unlock()

	if tap == nil {
		return
	}
	// Once per event, not once per recipient: a task nine people watch is one thing that
	// happened, and a receiving endpoint should be told about it once.
	//
	// Outside the lock and on its own goroutine, because the observer makes network calls to
	// servers we know nothing about. Holding the bus mutex through someone else's timeout would
	// stall live updates for everyone connected.
	recipients := append([]int64(nil), userIDs...)
	go tap(recipients, e)
}
