package httpapi

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSortByName(t *testing.T) {
	items := []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Name: "whoami"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "gitea"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "nextcloud"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "immich"}},
	}
	sortByName(items, func(o metav1.PartialObjectMetadata) string { return o.Name })
	want := []string{"gitea", "immich", "nextcloud", "whoami"}
	for i := range want {
		if items[i].Name != want[i] {
			t.Fatalf("order=%v want=%v", names(items), want)
		}
	}
}

func names(items []metav1.PartialObjectMetadata) []string {
	out := make([]string, 0, len(items))
	for i := range items {
		out = append(out, items[i].Name)
	}
	return out
}
