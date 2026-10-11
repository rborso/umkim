package reconcile

import (
	"testing"
	"time"
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

func TestSpokeACMEQueueRotateStaleHost(t *testing.T) {
	q := newSpokeACMEQueue()
	if !q.allow("c1", "a.example.com") {
		t.Fatal("first host should be allowed")
	}
	if q.allow("c1", "b.example.com") {
		t.Fatal("second host should wait")
	}
	// Force the active host to look stale, then rotate: the queued host must
	// become active and the stuck one must move to the back.
	q.mu.Lock()
	q.activeSince["c1"] = time.Now().Add(-10 * time.Minute)
	q.mu.Unlock()
	if q.staleFor("c1") < spokeACMEHostStaleAfter {
		t.Fatal("expected active host to look stale")
	}
	q.rotate("c1")
	if got := q.activeHost("c1"); got != "b.example.com" {
		t.Fatalf("active = %q, want b.example.com after rotation", got)
	}
	if q.staleFor("c1") >= spokeACMEHostStaleAfter {
		t.Fatal("rotated host should reset the staleness clock")
	}
	// a.example.com must still be reachable (moved to the back, not dropped).
	q.release("c1", "b.example.com")
	if got := q.activeHost("c1"); got != "a.example.com" {
		t.Fatalf("active = %q, want a.example.com (rotated to back)", got)
	}
}

func TestSpokeACMEQueueRotateWithoutPendingKeepsHost(t *testing.T) {
	q := newSpokeACMEQueue()
	if !q.allow("c1", "a.example.com") {
		t.Fatal("first host should be allowed")
	}
	q.mu.Lock()
	q.activeSince["c1"] = time.Now().Add(-10 * time.Minute)
	q.mu.Unlock()
	q.rotate("c1")
	if got := q.activeHost("c1"); got != "a.example.com" {
		t.Fatalf("active = %q, want a.example.com kept when nothing else is queued", got)
	}
	if q.staleFor("c1") >= spokeACMEHostStaleAfter {
		t.Fatal("rotation without pending should reset the staleness clock")
	}
}

