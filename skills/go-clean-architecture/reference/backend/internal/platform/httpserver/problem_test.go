package httpserver_test

import (
	"net/http"
	"testing"

	"example.com/app/internal/kernel/apperr"
	"example.com/app/internal/platform/httpserver"
)

// TestEveryKindHasItsOwnStatus: a new kind must get a status of its own; falling through to 500
// would turn a client's error into "internal".
func TestEveryKindHasItsOwnStatus(t *testing.T) {
	owner := map[int]apperr.Kind{}
	for _, k := range apperr.Kinds() {
		status := httpserver.StatusOf(k)
		if prev, taken := owner[status]; taken {
			t.Errorf("kinds %v and %v both map to %d", prev, k, status)
		}
		owner[status] = k
		if (status == http.StatusInternalServerError) != (k == apperr.KindInternal) {
			t.Errorf("StatusOf(%v) = %d: only the internal kind is a 500", k, status)
		}
	}
}
