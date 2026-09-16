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

func TestCanonicalURLNormalizesCommonArticleVariants(t *testing.T) {
	got := CanonicalURL(" https://EXAMPLE.com:443/a/./b/?z=2&utm_source=news&x=1#section ")
	want := "https://example.com/a/b/?x=1&z=2"
	if got != want {
		t.Fatalf("canonical URL: got %q want %q", got, want)
	}

	if RawID("https://example.com/article?utm_campaign=a#comments") != RawID("https://EXAMPLE.com:443/article") {
		t.Fatal("article variants should produce one raw id")
	}
	if RawID("https://example.com/article?id=1") == RawID("https://example.com/article?id=2") {
		t.Fatal("business query params must remain distinct")
	}
}

func TestCanonicalURLKeepsStableNonURLSeeds(t *testing.T) {
	seed := "aihot:b1"
	if got := CanonicalURL(seed); got != seed {
		t.Fatalf("non-URL seed changed: got %q want %q", got, seed)
	}
	if RawID(seed) != RawID("aihot:b1") {
		t.Fatal("non-URL seed should stay stable")
	}
}

func TestDefaultSourceRole(t *testing.T) {
	if got := DefaultSourceRole(""); got != SourceRoleDiscovery {
		t.Fatalf("empty role should default to discovery, got %q", got)
	}
	if got := DefaultSourceRole(SourceRoleOfficial); got != SourceRoleOfficial {
		t.Fatalf("explicit role should pass through, got %q", got)
	}
}

func TestSourceKindAndRoleValidation(t *testing.T) {
	for _, role := range []string{SourceRoleOfficial, SourceRoleProfessional, SourceRoleDiscovery} {
		if !ValidSourceRole(role) {
			t.Fatalf("role %q should be valid", role)
		}
	}
	if ValidSourceRole("vendor") {
		t.Fatal("unknown role should be invalid")
	}
	for _, kind := range []string{SourceKindRSS, SourceKindHTML, SourceKindMP, SourceKindAIHOT} {
		if !ValidSourceKind(kind) {
			t.Fatalf("kind %q should be valid", kind)
		}
	}
	if ValidSourceKind("email") {
		t.Fatal("unknown kind should be invalid")
	}
}
