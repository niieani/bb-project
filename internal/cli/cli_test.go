package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestStripGlobalFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        []string
		wantArgs  []string
		wantQuiet bool
		wantHelp  bool
	}{
		{name: "no flags", in: []string{"sync"}, wantArgs: []string{"sync"}, wantQuiet: false, wantHelp: false},
		{name: "quiet before command", in: []string{"--quiet", "sync"}, wantArgs: []string{"sync"}, wantQuiet: true, wantHelp: false},
		{name: "quiet after command", in: []string{"sync", "--quiet", "--push"}, wantArgs: []string{"sync", "--push"}, wantQuiet: true, wantHelp: false},
		{name: "short quiet", in: []string{"-q", "status"}, wantArgs: []string{"status"}, wantQuiet: true, wantHelp: false},
		{name: "long help", in: []string{"--help"}, wantArgs: []string{}, wantQuiet: false, wantHelp: true},
		{name: "short help", in: []string{"-h", "sync"}, wantArgs: []string{"sync"}, wantQuiet: false, wantHelp: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotArgs, gotQuiet, gotHelp := stripGlobalFlags(tt.in)
			if gotQuiet != tt.wantQuiet {
				t.Fatalf("quiet = %v, want %v", gotQuiet, tt.wantQuiet)
			}
			if gotHelp != tt.wantHelp {
				t.Fatalf("help = %v, want %v", gotHelp, tt.wantHelp)
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

func TestParseConfigArgs(t *testing.T) {
	t.Parallel()

	if err := parseConfigArgs(nil); err != nil {
		t.Fatalf("parseConfigArgs(nil) error = %v", err)
	}

	if err := parseConfigArgs([]string{"--unknown"}); err == nil {
		t.Fatal("expected error for unknown config arg")
	}
}

func TestRunHelpCommandShowsGeneralHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
	out := stdout.String()
	mustContain(t, out, "Usage:")
	mustContain(t, out, "bb [global flags] <command> [args]")
	mustContain(t, out, "Commands:")
	mustContain(t, out, "help")
	mustContain(t, out, "Examples:")
}

func TestRunHelpCommandShowsCommandHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"help", "sync"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
	out := stdout.String()
	mustContain(t, out, "Usage: bb sync")
	mustContain(t, out, "--dry-run")
	mustContain(t, out, "--notify")
}

func TestRunHelpFlagShowsCommandHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"sync", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
	mustContain(t, stdout.String(), "Usage: bb sync")
}

func TestRunUnknownHelpTopic(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"help", "not-a-command"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	errOut := stderr.String()
	mustContain(t, errOut, "unknown help topic")
	mustContain(t, errOut, "Usage:")
}

func mustContain(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}
