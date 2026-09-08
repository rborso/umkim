package connectors

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const maxRevisions = 50

// RevisionScope identifies what changed in connector inventory.
type RevisionScope string

const (
	RevisionScopeCatalog RevisionScope = "catalog"
	RevisionScopeGuest   RevisionScope = "guest"
	RevisionScopeFile    RevisionScope = "file"
	RevisionScopeImport    RevisionScope = "import"
	RevisionScopeSync      RevisionScope = "sync"
	RevisionScopeProvision RevisionScope = "provision"
)

// Revision is one append-only inventory change record.
type Revision struct {
	ID      string          `json:"id"`
	TS      string          `json:"ts"`
	Actor   string          `json:"actor"`
	Scope   RevisionScope   `json:"scope"`
	Summary string          `json:"summary"`
	Before  json.RawMessage `json:"before,omitempty"`
	After   json.RawMessage `json:"after,omitempty"`
}

// RevisionLog holds the ring buffer in revisions.json.
type RevisionLog struct {
	Items []Revision `json:"items"`
}

func newRevisionID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func FormatActor(kind, name string) string {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if kind == "" && name == "" {
		return "system"
	}
	if kind == "" {
		return name
	}
	return kind + "/" + name
}

// AppendRevision prepends a revision and trims to maxRevisions.
func AppendRevision(log RevisionLog, rev Revision) RevisionLog {
	if rev.ID == "" {
		rev.ID = newRevisionID()
	}
	if rev.TS == "" {
		rev.TS = time.Now().UTC().Format(time.RFC3339)
	}
	items := append([]Revision{rev}, log.Items...)
	if len(items) > maxRevisions {
		items = items[:maxRevisions]
	}
	return RevisionLog{Items: items}
}

func decodeRevisions(raw string) (RevisionLog, error) {
	if strings.TrimSpace(raw) == "" {
		return RevisionLog{}, nil
	}
	var log RevisionLog
	if err := json.Unmarshal([]byte(raw), &log); err != nil {
		return RevisionLog{}, fmt.Errorf("invalid revisions.json: %w", err)
	}
	return log, nil
}

func encodeRevisions(log RevisionLog) (string, error) {
	raw, err := json.Marshal(log)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func findRevision(log RevisionLog, id string) (Revision, bool) {
	for _, r := range log.Items {
		if r.ID == id {
			return r, true
		}
	}
	return Revision{}, false
}
