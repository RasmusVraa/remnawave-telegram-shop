package handler

import "testing"

// Payload без «&» раньше ронял обработчик успешной оплаты (payload[1]).
func TestParseStarsPayload(t *testing.T) {
	id, user, err := parseStarsPayload("42&ivan")
	if err != nil || id != 42 || user != "ivan" {
		t.Fatalf("got %d %q %v", id, user, err)
	}
	id, user, err = parseStarsPayload("42")
	if err != nil || id != 42 || user != "" {
		t.Fatalf("payload without username: got %d %q %v", id, user, err)
	}
	for _, bad := range []string{"", "&ivan", "abc&ivan", "-1&x", "0"} {
		if _, _, err := parseStarsPayload(bad); err == nil {
			t.Errorf("payload %q accepted", bad)
		}
	}
}
