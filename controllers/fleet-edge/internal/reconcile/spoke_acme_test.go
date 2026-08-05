package reconcile

import (
	"testing"
)

func TestSpokeACMEQueueSerializes(t *testing.T) {
	q := newSpokeACMEQueue()
	if !q.allow("c1", "a.example.com") {
		t.Fatal("first host should be allowed")
	}
	if q.allow("c1", "b.example.com") {
		t.Fatal("second host should wait")
	}
	q.release("c1", "a.example.com")
	if !q.allow("c1", "b.example.com") {
		t.Fatal("next host should be allowed after release")
	}
}
