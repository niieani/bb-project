package app

import "testing"

func TestParseFixSyncStrategy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		want      FixSyncStrategy
		shouldErr bool
	}{
		{name: "default empty", raw: "", want: FixSyncStrategyRebase},
		{name: "rebase", raw: "rebase", want: FixSyncStrategyRebase},
		{name: "merge", raw: "merge", want: FixSyncStrategyMerge},
		{name: "trim and lower", raw: " MERGE ", want: FixSyncStrategyMerge},
		{name: "invalid", raw: "squash", shouldErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseFixSyncStrategy(tt.raw)
			if tt.shouldErr {
				if err == nil {
					t.Fatalf("ParseFixSyncStrategy(%q) expected error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFixSyncStrategy(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseFixSyncStrategy(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
