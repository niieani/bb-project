package app

import (
	"testing"

	"bb-project/internal/domain"
)

func TestEffectiveAutoPushForObservedBranch(t *testing.T) {
	t.Parallel()

	trueValue := true
	falseValue := false

	tests := []struct {
		name          string
		autoPush      bool
		catalog       domain.Catalog
		visibility    domain.Visibility
		branch        string
		defaultBranch string
		want          bool
	}{
		{
			name:       "auto push disabled in metadata",
			autoPush:   false,
			visibility: domain.VisibilityPrivate,
			branch:     "main",
			want:       false,
		},
		{
			name:       "non-default branch keeps auto push enabled",
			autoPush:   true,
			visibility: domain.VisibilityPublic,
			branch:     "feature/x",
			want:       true,
		},
		{
			name:       "private default branch defaults to allowed",
			autoPush:   true,
			visibility: domain.VisibilityPrivate,
			branch:     "main",
			want:       true,
		},
		{
			name:       "public default branch defaults to blocked",
			autoPush:   true,
			visibility: domain.VisibilityPublic,
			branch:     "main",
			want:       false,
		},
		{
			name:       "private override blocks",
			autoPush:   true,
			catalog:    domain.Catalog{AllowAutoPushDefaultBranchPrivate: &falseValue},
			visibility: domain.VisibilityPrivate,
			branch:     "main",
			want:       false,
		},
		{
			name:       "public override allows",
			autoPush:   true,
			catalog:    domain.Catalog{AllowAutoPushDefaultBranchPublic: &trueValue},
			visibility: domain.VisibilityPublic,
			branch:     "main",
			want:       true,
		},
		{
			name:          "detected default branch name is honored",
			autoPush:      true,
			visibility:    domain.VisibilityPrivate,
			branch:        "trunk",
			defaultBranch: "trunk",
			want:          true,
		},
		{
			name:          "detected default branch can block public",
			autoPush:      true,
			visibility:    domain.VisibilityPublic,
			branch:        "trunk",
			defaultBranch: "trunk",
			want:          false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := effectiveAutoPushForObservedBranch(tt.autoPush, tt.catalog, tt.visibility, tt.branch, tt.defaultBranch)
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
