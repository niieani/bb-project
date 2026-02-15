package app

import (
	"testing"

	"bb-project/internal/domain"
)

func TestEffectiveAutoPushForObservedBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mode          domain.AutoPushMode
		branch        string
		defaultBranch string
		want          bool
	}{
		{
			name:   "auto push disabled in metadata",
			mode:   domain.AutoPushModeDisabled,
			branch: "main",
			want:   false,
		},
		{
			name:   "non-default branch keeps auto push enabled",
			mode:   domain.AutoPushModeEnabled,
			branch: "feature/x",
			want:   true,
		},
		{
			name:   "default branch blocked for non-default mode",
			mode:   domain.AutoPushModeEnabled,
			branch: "main",
			want:   false,
		},
		{
			name:   "default branch allowed for include-default mode",
			mode:   domain.AutoPushModeIncludeDefaultBranch,
			branch: "main",
			want:   true,
		},
		{
			name:          "detected default branch name is honored",
			mode:          domain.AutoPushModeEnabled,
			branch:        "trunk",
			defaultBranch: "trunk",
			want:          false,
		},
		{
			name:          "detected default branch allows include-default mode",
			mode:          domain.AutoPushModeIncludeDefaultBranch,
			branch:        "trunk",
			defaultBranch: "trunk",
			want:          true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := effectiveAutoPushForObservedBranch(tt.mode, tt.branch, tt.defaultBranch)
			if got != tt.want {
				t.Fatalf("effectiveAutoPushForObservedBranch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDefaultBranchFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		branch        string
		defaultBranch string
		want          bool
	}{
		{name: "explicit default matches", branch: "trunk", defaultBranch: "trunk", want: true},
		{name: "explicit default mismatch", branch: "main", defaultBranch: "trunk", want: false},
		{name: "fallback accepts main", branch: "main", want: true},
		{name: "fallback accepts master", branch: "master", want: true},
		{name: "fallback rejects feature", branch: "feature/x", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isDefaultBranch(tt.branch, tt.defaultBranch); got != tt.want {
				t.Fatalf("isDefaultBranch(%q,%q) = %v, want %v", tt.branch, tt.defaultBranch, got, tt.want)
			}
		})
	}
}
