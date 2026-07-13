package detailpage

import (
	"testing"

	"aihot-server/internal/items"
)

func TestToViewModelScoreLetterAndTier(t *testing.T) {
	cases := []struct {
		score      int
		wantLetter string
	}{
		{5, "S"}, {4, "A"}, {3, "B"}, {2, "C"}, {1, "D"},
	}
	for _, c := range cases {
		s := c.score
		it := &items.Item{ID: "x", Title: "标题", URL: "https://ex.com/x", Permalink: "/items/x", Source: "S", Score: &s, Selected: true}
		vm := toViewModel(it)
		if !vm.HasScore {
			t.Fatalf("score=%d: HasScore should be true", c.score)
		}
		if vm.Score != c.wantLetter || vm.Tier != c.wantLetter {
			t.Fatalf("score=%d: got Score=%q Tier=%q, want %q", c.score, vm.Score, vm.Tier, c.wantLetter)
		}
	}
}

func TestToViewModelNoScore(t *testing.T) {
	it := &items.Item{ID: "x", Title: "t", URL: "https://ex.com/x", Permalink: "/items/x", Source: "S"}
	vm := toViewModel(it)
	if vm.HasScore || vm.Score != "" || vm.Tier != "" {
		t.Fatalf("no score: HasScore=%v Score=%q Tier=%q", vm.HasScore, vm.Score, vm.Tier)
	}
}

func TestScoreLetterClamps(t *testing.T) {
	if scoreLetter(9) != "S" || scoreLetter(0) != "D" {
		t.Fatalf("clamp: 9=%q 0=%q", scoreLetter(9), scoreLetter(0))
	}
}
