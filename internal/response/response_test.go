package response

import (
	"encoding/json"
	"strings"
	"testing"
)

// An empty list must reach the client as "data": [] (omitempty would drop it), while message-only
// responses still have no data field.
func TestDataOmittedOnlyWhenNil(t *testing.T) {
	empty, _ := json.Marshal(Response[[]string]{Data: []string{}})
	if !strings.Contains(string(empty), `"data":[]`) {
		t.Fatalf("empty list dropped: %s", empty)
	}
	none, _ := json.Marshal(Response[any]{})
	if strings.Contains(string(none), `"data"`) {
		t.Fatalf("nil data present: %s", none)
	}
}
