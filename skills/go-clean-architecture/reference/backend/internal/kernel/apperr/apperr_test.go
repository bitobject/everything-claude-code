package apperr_test

import (
	"testing"

	"example.com/app/internal/kernel/apperr"
)

// TestKindsListsEveryKind: the kinds are iota constants, so the value after the last one in
// Kinds must be unnamed. A kind added to the constants but not to Kinds (or String) fails here.
func TestKindsListsEveryKind(t *testing.T) {
	kinds := apperr.Kinds()
	seen := map[string]bool{}
	for i, k := range kinds {
		if int(k) != i {
			t.Errorf("Kinds()[%d] = %d, want the constants in order", i, k)
		}
		if name := k.String(); name == "unknown" || seen[name] {
			t.Errorf("kind %d is named %q: every kind needs a name of its own", k, name)
		} else {
			seen[name] = true
		}
	}
	if next := apperr.Kind(len(kinds)); next.String() != "unknown" {
		t.Errorf("kind %d is named %q but missing from Kinds()", next, next.String())
	}
}
