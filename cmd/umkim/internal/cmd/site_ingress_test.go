package cmd

import (
	"errors"
	"testing"
	"time"
)

func siteDocWith(ids ...string) *siteIngressDoc {
	doc := &siteIngressDoc{}
	for _, id := range ids {
		doc.Spec.Edge.Ingress = append(doc.Spec.Edge.Ingress, siteIngressRow{ID: id, Method: "port_forward"})
	}
	return doc
}

// A delete that landed must not be reported as a failure just because the first
// re-read still answered with the pre-patch list.
func TestWaitSiteIngressSettlesOnStaleRead(t *testing.T) {
	calls := 0
	fetch := func(string) (*siteIngressDoc, []byte, error) {
		calls++
		if calls < 3 {
			return siteDocWith("a", "b"), nil, nil
		}
		return siteDocWith("a"), nil, nil
	}
	doc, err := waitSiteIngressWith("homelab", fetch,
		func(d *siteIngressDoc) bool { return len(d.Spec.Edge.Ingress) == 1 },
		5, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Edge.Ingress) != 1 || doc.Spec.Edge.Ingress[0].ID != "a" {
		t.Fatalf("doc = %+v", doc.Spec.Edge.Ingress)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

// A genuine mismatch still returns the last document so the caller reports it.
func TestWaitSiteIngressReturnsLastDocumentWhenNeverSettled(t *testing.T) {
	fetch := func(string) (*siteIngressDoc, []byte, error) {
		return siteDocWith("a", "b"), nil, nil
	}
	doc, err := waitSiteIngressWith("homelab", fetch,
		func(d *siteIngressDoc) bool { return len(d.Spec.Edge.Ingress) == 1 },
		3, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Edge.Ingress) != 2 {
		t.Fatalf("doc = %+v, want the last read", doc.Spec.Edge.Ingress)
	}
}

func TestWaitSiteIngressSurfacesFetchError(t *testing.T) {
	want := errors.New("boom")
	fetch := func(string) (*siteIngressDoc, []byte, error) { return nil, nil, want }
	if _, err := waitSiteIngressWith("homelab", fetch, func(*siteIngressDoc) bool { return true }, 2, time.Millisecond); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
