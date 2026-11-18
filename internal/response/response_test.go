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

// reason is optional and additive: present only when the error carries one, so older clients that
// read code/message/errors keep working.
func TestReasonOnlyWhenSet(t *testing.T) {
	tests := []struct {
		name string
		r    Response[any]
		want string
		has  bool
	}{
		{"with reason", Response[any]{Code: "BAD_REQUEST", Reason: "voucher_expired"}, `"reason":"voucher_expired"`, true},
		{"without reason", Response[any]{Code: "BAD_REQUEST"}, `"reason"`, false},
	}
	for _, tt := range tests {
		b, _ := json.Marshal(tt.r)
		if strings.Contains(string(b), tt.want) != tt.has {
			t.Errorf("%s: %s", tt.name, b)
		}
	}
}
