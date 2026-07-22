// Package events — in-memory шина событий для SSE (подписка по user id).
package events

import "sync"

type Event struct {
	Type string `json:"type"` // task.updated | comment.created | notification | pulse.changed | announcement
	Data any    `json:"data,omitempty"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[int64]map[chan Event]struct{}
}

func New() *Bus { return &Bus{subs: map[int64]map[chan Event]struct{}{}} }

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

// Publish — неблокирующая доставка; медленные подписчики пропускают событие.
func (b *Bus) Publish(userIDs []int64, e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range userIDs {
		for ch := range b.subs[id] {
			select {
			case ch <- e:
			default:
			}
		}
	}
}
