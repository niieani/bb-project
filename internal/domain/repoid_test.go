package domain

import "testing"

func TestNormalizeOriginToRepoID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
		want   string
	}{
		{name: "ssh github", origin: "git@github.com:You/BB-Project.git", want: "github.com/you/bb-project"},
		{name: "https github", origin: "https://github.com/You/BB-Project.git", want: "github.com/you/bb-project"},
		{name: "ssh with ssh protocol", origin: "ssh://git@github.com/You/BB-Project.git", want: "github.com/you/bb-project"},
		{name: "trim trailing slash", origin: "https://github.com/You/BB-Project/", want: "github.com/you/bb-project"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeOriginToRepoID(tt.origin)
			if err != nil {
				t.Fatalf("NormalizeOriginToRepoID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeOriginToRepoID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeOriginToRepoIDError(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeOriginToRepoID("not a url"); err == nil {
		t.Fatal("expected error for invalid origin")
	}
}
