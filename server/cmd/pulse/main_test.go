package main

import (
	"strings"
	"testing"

	"aihot-server/internal/pulse"
)

func TestPulseSummaryError(t *testing.T) {
	tests := []struct {
		name string
		sum  pulse.Summary
		want string
	}{
		{name: "success", sum: pulse.Summary{}, want: ""},
		{name: "source errors", sum: pulse.Summary{SourceErrors: 2}, want: "source_errors=2"},
		{name: "enrichment failures", sum: pulse.Summary{Failed: 3}, want: "failed=3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pulseSummaryError(tt.sum)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("pulseSummaryError() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("pulseSummaryError() = %v, want substring %q", err, tt.want)
			}
		})
	}
}
