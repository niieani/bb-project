package domain

import "testing"

func TestParseAutoPushMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    AutoPushMode
		wantErr bool
	}{
		{name: "false", raw: "false", want: AutoPushModeDisabled},
		{name: "true", raw: "true", want: AutoPushModeEnabled},
		{name: "include default", raw: "include-default-branch", want: AutoPushModeIncludeDefaultBranch},
		{name: "trim and lowercase", raw: "  INCLUDE-DEFAULT-BRANCH  ", want: AutoPushModeIncludeDefaultBranch},
		{name: "invalid", raw: "maybe", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAutoPushMode(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAutoPushMode() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseAutoPushMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeAutoPushMode(t *testing.T) {
	t.Parallel()

	if got := NormalizeAutoPushMode(""); got != AutoPushModeDisabled {
		t.Fatalf("NormalizeAutoPushMode(\"\") = %q, want %q", got, AutoPushModeDisabled)
	}
	if got := NormalizeAutoPushMode(" TRUE "); got != AutoPushModeEnabled {
		t.Fatalf("NormalizeAutoPushMode(\" TRUE \") = %q, want %q", got, AutoPushModeEnabled)
	}
}
