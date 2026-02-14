package gitx

import "testing"

func TestLooksLikePushAccessDenied(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{name: "permission denied", msg: "remote: Permission denied to user/repo.", want: true},
		{name: "write access not granted", msg: "remote: Write access to repository not granted.", want: true},
		{name: "network timeout", msg: "fatal: unable to access remote: timeout", want: false},
		{name: "empty", msg: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikePushAccessDenied(tt.msg); got != tt.want {
				t.Fatalf("looksLikePushAccessDenied() = %v, want %v", got, tt.want)
			}
		})
	}
}
