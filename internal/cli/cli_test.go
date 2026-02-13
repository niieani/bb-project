package cli

import "testing"

func TestStripGlobalFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        []string
		wantArgs  []string
		wantQuiet bool
	}{
		{name: "no flags", in: []string{"sync"}, wantArgs: []string{"sync"}, wantQuiet: false},
		{name: "quiet before command", in: []string{"--quiet", "sync"}, wantArgs: []string{"sync"}, wantQuiet: true},
		{name: "quiet after command", in: []string{"sync", "--quiet", "--push"}, wantArgs: []string{"sync", "--push"}, wantQuiet: true},
		{name: "short quiet", in: []string{"-q", "status"}, wantArgs: []string{"status"}, wantQuiet: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotArgs, gotQuiet := stripGlobalFlags(tt.in)
			if gotQuiet != tt.wantQuiet {
				t.Fatalf("quiet = %v, want %v", gotQuiet, tt.wantQuiet)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("args len = %d, want %d (args=%v)", len(gotArgs), len(tt.wantArgs), gotArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}
