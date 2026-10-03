package cmd

import (
	"testing"
	"time"
)

func TestRetryOnHTTP409EventuallySucceeds(t *testing.T) {
	calls := 0
	raw, code, err := retryOnHTTP409WithBackoff(func() ([]byte, int, error) {
		calls++
		if calls < 3 {
			return []byte(`{"error":"site provision action already running"}`), 409, nil
		}
		return []byte(`{"id":"action-1"}`), 202, nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if code != 202 {
		t.Fatalf("code=%d", code)
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
	if string(raw) != `{"id":"action-1"}` {
		t.Fatalf("raw=%s", raw)
	}
}

func TestRetryOnHTTP409Exhausted(t *testing.T) {
	_, code, err := retryOnHTTP409WithBackoff(func() ([]byte, int, error) {
		return []byte(`{"error":"inventory sync already in progress"}`), 409, nil
	}, 2, time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if code != 409 {
		t.Fatalf("code=%d", code)
	}
}
