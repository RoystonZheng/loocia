package publicapi

import (
	"testing"
	"time"

	"aihot-server/internal/items"
)

func TestCursorRoundTrips(t *testing.T) {
	sk := time.Date(2026, 5, 7, 12, 30, 0, 0, time.UTC)
	in := &items.Cursor{SortKey: sk, ID: "abc123"}
	tok := encodeCursor(in)
	if tok == "" {
		t.Fatal("encodeCursor returned empty")
	}
	out := decodeCursor(tok)
	if out == nil {
		t.Fatal("decodeCursor returned nil for a valid token")
	}
	if !out.SortKey.Equal(sk) || out.ID != "abc123" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestEncodeNilCursorIsEmpty(t *testing.T) {
	if encodeCursor(nil) != "" {
		t.Fatal("nil cursor should encode to empty string")
	}
}

func TestDecodeGarbageReturnsNil(t *testing.T) {
	for _, bad := range []string{"", "not-base64!!!", "YWJjZGVm", "e30=" /* {} */, "bm90anNvbg=="} {
		if got := decodeCursor(bad); got != nil {
			t.Fatalf("decodeCursor(%q) = %+v, want nil (lenient → first page)", bad, got)
		}
	}
}
