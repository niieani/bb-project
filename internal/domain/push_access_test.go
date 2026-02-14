package domain

import "testing"

func TestParsePushAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    PushAccess
		wantErr bool
	}{
		{name: "empty defaults unknown", raw: "", want: PushAccessUnknown},
		{name: "unknown", raw: "unknown", want: PushAccessUnknown},
		{name: "read write", raw: "read_write", want: PushAccessReadWrite},
		{name: "read only", raw: "read_only", want: PushAccessReadOnly},
		{name: "trim and lowercase", raw: "  READ_WRITE  ", want: PushAccessReadWrite},
		{name: "invalid", raw: "admin", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParsePushAccess(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePushAccess() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParsePushAccess() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePushAccess(t *testing.T) {
	t.Parallel()

	if got := NormalizePushAccess(PushAccessReadWrite); got != PushAccessReadWrite {
		t.Fatalf("NormalizePushAccess(read_write) = %q", got)
	}
	if got := NormalizePushAccess(PushAccessReadOnly); got != PushAccessReadOnly {
		t.Fatalf("NormalizePushAccess(read_only) = %q", got)
	}
	if got := NormalizePushAccess(""); got != PushAccessUnknown {
		t.Fatalf("NormalizePushAccess(empty) = %q, want unknown", got)
	}
	if got := NormalizePushAccess(PushAccess("invalid")); got != PushAccessUnknown {
		t.Fatalf("NormalizePushAccess(invalid) = %q, want unknown", got)
	}
}
