package sse

import "sync"

// Event represents a server-sent event with a named type and HTML data.
type Event struct {
	Name string
	Data string
}

// Broker manages SSE subscriptions and event delivery per lift.
type Broker struct {
	mu          sync.Mutex
	subscribers map[int64]map[chan Event]struct{}
	processing  map[int64]struct{}
	lastState   map[int64]map[string]Event
}

// NewBroker creates a new SSE broker.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[int64]map[chan Event]struct{}),
		processing:  make(map[int64]struct{}),
		lastState:   make(map[int64]map[string]Event),
	}
}

// Subscribe creates a new subscription for a lift's events.
// Cached state is immediately sent to the new subscriber.
func (b *Broker) Subscribe(liftID int64) chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 16)
	if b.subscribers[liftID] == nil {
		b.subscribers[liftID] = make(map[chan Event]struct{})
	}
	b.subscribers[liftID][ch] = struct{}{}

	// Replay cached state so new subscribers see current progress.
	if cached, ok := b.lastState[liftID]; ok {
		for _, event := range cached {
			ch <- event
		}
	}

	return ch
}

// Unsubscribe removes a subscription and closes the channel.
func (b *Broker) Unsubscribe(liftID int64, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subs, ok := b.subscribers[liftID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(b.subscribers, liftID)
		}
	}
	close(ch)
}

// Publish sends an event to all subscribers for a lift and caches it.
func (b *Broker) Publish(liftID int64, event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.lastState[liftID] == nil {
		b.lastState[liftID] = make(map[string]Event)
	}
	b.lastState[liftID][event.Name] = event

	for ch := range b.subscribers[liftID] {
		select {
		case ch <- event:
		default:
			// Drop if subscriber can't keep up.
		}
	}
}

// StartProcessing marks a lift as actively being processed.
// Clears any cached state from a previous pipeline run.
func (b *Broker) StartProcessing(liftID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.processing[liftID] = struct{}{}
	delete(b.lastState, liftID)
}

// StopProcessing marks a lift as no longer being processed.
// Cached state is preserved for late-connecting subscribers.
func (b *Broker) StopProcessing(liftID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.processing, liftID)
}

// IsProcessing returns whether a lift has an active pipeline.
func (b *Broker) IsProcessing(liftID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.processing[liftID]
	return ok
}
