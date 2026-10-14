package httpapi

import "sort"

// The controller-runtime cached client returns List items in map order, so a
// list response would otherwise shuffle between identical calls. The console
// shows that as a re-ordering list, so every list handler sorts before it
// answers.

// sortByName orders a slice in place by its name key.
func sortByName[T any](items []T, name func(T) string) {
	sort.SliceStable(items, func(i, j int) bool { return name(items[i]) < name(items[j]) })
}
