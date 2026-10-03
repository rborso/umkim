package reconcile

import "testing"

func TestStagingPromotionAction(t *testing.T) {
	const host = "whoami.homelab.otthol.hu"
	cases := []struct {
		name    string
		evicted string
		served  bool
		init    bool
		want    string
	}{
		{name: "already evicted", evicted: host, served: true, init: true, want: "noop"},
		{name: "wait for init container", evicted: "", served: true, init: false, want: "wait-init"},
		{name: "restart while staging cert is served", evicted: "", served: true, init: true, want: "restart"},
		{name: "mark after wipe without another restart", evicted: "", served: false, init: true, want: "mark"},
		{name: "new host after a previous eviction", evicted: "auth.homelab.otthol.hu", served: true, init: true, want: "restart"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stagingPromotionAction(tc.evicted, host, tc.served, tc.init)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
