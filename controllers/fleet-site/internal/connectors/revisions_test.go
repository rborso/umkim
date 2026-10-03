package connectors

import (
	"encoding/json"
	"testing"
)

func TestAppendRevisionRing(t *testing.T) {
	log := RevisionLog{}
	for i := 0; i < 60; i++ {
		log = AppendRevision(log, Revision{Summary: "change"})
	}
	if len(log.Items) != maxRevisions {
		t.Fatalf("len=%d want %d", len(log.Items), maxRevisions)
	}
	if log.Items[0].ID == "" || log.Items[0].TS == "" {
		t.Fatalf("missing id/ts on newest revision: %+v", log.Items[0])
	}
}

func TestDecodeRevisionsEmpty(t *testing.T) {
	log, err := decodeRevisions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(log.Items) != 0 {
		t.Fatalf("items=%d", len(log.Items))
	}
}

func TestRevisionBeforeAfterJSON(t *testing.T) {
	before := json.RawMessage(`{"a":1}`)
	after := json.RawMessage(`{"a":2}`)
	log := AppendRevision(RevisionLog{}, Revision{
		Actor:   "user/alice",
		Scope:   RevisionScopeGuest,
		Summary: "patch",
		Before:  before,
		After:   after,
	})
	if log.Items[0].Actor != "user/alice" {
		t.Fatalf("actor=%q", log.Items[0].Actor)
	}
}
