package sse

import (
	"sync"
	"testing"
	"time"
)

func TestSubscribePublish(t *testing.T) {
	b := NewBroker()
	ch := b.Subscribe(1)

	b.Publish(1, Event{Name: "test", Data: "hello"})

	select {
	case e := <-ch:
		if e.Name != "test" || e.Data != "hello" {
			t.Errorf("unexpected event: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	b.Unsubscribe(1, ch)
}

func TestMultipleSubscribers(t *testing.T) {
	b := NewBroker()
	ch1 := b.Subscribe(1)
	ch2 := b.Subscribe(1)

	b.Publish(1, Event{Name: "test", Data: "hello"})

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Data != "hello" {
				t.Errorf("subscriber %d: unexpected data: %s", i, e.Data)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}

	b.Unsubscribe(1, ch1)
	b.Unsubscribe(1, ch2)
}

func TestLiftIsolation(t *testing.T) {
	b := NewBroker()
	ch1 := b.Subscribe(1)
	ch2 := b.Subscribe(2)

	b.Publish(1, Event{Name: "test", Data: "for-1"})

	select {
	case e := <-ch1:
		if e.Data != "for-1" {
			t.Errorf("unexpected data: %s", e.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event on ch1")
	}

	select {
	case <-ch2:
		t.Error("ch2 should not receive events for lift 1")
	case <-time.After(50 * time.Millisecond):
		// Expected: no event for different lift.
	}

	b.Unsubscribe(1, ch1)
	b.Unsubscribe(2, ch2)
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	b := NewBroker()
	ch := b.Subscribe(1)
	b.Unsubscribe(1, ch)

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe")
	}
}

func TestProcessingLifecycle(t *testing.T) {
	b := NewBroker()

	if b.IsProcessing(1) {
		t.Error("should not be processing initially")
	}

	b.StartProcessing(1)
	if !b.IsProcessing(1) {
		t.Error("should be processing after StartProcessing")
	}

	b.StopProcessing(1)
	if b.IsProcessing(1) {
		t.Error("should not be processing after StopProcessing")
	}
}

func TestReplayOnSubscribe(t *testing.T) {
	b := NewBroker()

	// Publish before any subscriber connects.
	b.Publish(1, Event{Name: "stages", Data: "html1"})
	b.Publish(1, Event{Name: "status", Data: "html2"})

	ch := b.Subscribe(1)

	events := make(map[string]string)
	for i := 0; i < 2; i++ {
		select {
		case e := <-ch:
			events[e.Name] = e.Data
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for replay")
		}
	}

	if events["stages"] != "html1" {
		t.Errorf("expected stages=html1, got %s", events["stages"])
	}
	if events["status"] != "html2" {
		t.Errorf("expected status=html2, got %s", events["status"])
	}

	b.Unsubscribe(1, ch)
}

func TestStartProcessingClearsOldState(t *testing.T) {
	b := NewBroker()

	b.Publish(1, Event{Name: "old", Data: "stale"})
	b.StartProcessing(1)

	ch := b.Subscribe(1)

	// Should not receive old cached events.
	select {
	case e := <-ch:
		t.Errorf("expected no replay after StartProcessing, got: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// Expected: no replay.
	}

	b.Unsubscribe(1, ch)
}

func TestConcurrentAccess(t *testing.T) {
	b := NewBroker()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			ch := b.Subscribe(id)
			b.Publish(id, Event{Name: "test", Data: "data"})
			<-ch
			b.Unsubscribe(id, ch)
		}(int64(i))
	}

	wg.Wait()
}
