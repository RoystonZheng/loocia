package ingest

import "testing"

func TestRawIDStableAndURLUnique(t *testing.T) {
	a1 := RawID("https://ex.com/a")
	a2 := RawID("https://ex.com/a")
	b := RawID("https://ex.com/b")

	if a1 != a2 {
		t.Fatalf("RawID not stable: %q vs %q", a1, a2)
	}
	if a1 == b {
		t.Fatal("distinct URLs produced the same id")
	}
	if len(a1) != 64 {
		t.Fatalf("want 64 hex chars (sha256), got %d", len(a1))
	}
}
