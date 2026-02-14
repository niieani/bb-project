package app

import (
	"errors"
	"testing"
)

func TestRunFixInteractiveWithMutedLogsMutesAndRestoresVerbose(t *testing.T) {
	t.Parallel()

	app := &App{
		Verbose: true,
		IsInteractiveTerminal: func() bool {
			return true
		},
	}

	var verboseDuringCall bool
	app.runFixInteractiveFn = func(_ []string, _ bool) (int, error) {
		verboseDuringCall = app.Verbose
		return 0, nil
	}

	code, err := app.runFix(FixOptions{})
	if err != nil {
		t.Fatalf("runFix returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("runFix code = %d, want 0", code)
	}
	if verboseDuringCall {
		t.Fatal("expected verbose logging to be disabled during interactive tui startup")
	}
	if !app.Verbose {
		t.Fatal("expected verbose setting to be restored after interactive startup")
	}
}

func TestRunFixInteractiveWithMutedLogsPreservesQuietMode(t *testing.T) {
	t.Parallel()

	app := &App{
		Verbose: false,
		IsInteractiveTerminal: func() bool {
			return true
		},
	}

	var verboseDuringCall bool
	app.runFixInteractiveFn = func(_ []string, _ bool) (int, error) {
		verboseDuringCall = app.Verbose
		return 0, nil
	}

	code, err := app.runFix(FixOptions{})
	if err != nil {
		t.Fatalf("runFix returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("runFix code = %d, want 0", code)
	}
	if verboseDuringCall {
		t.Fatal("expected verbose logging to remain disabled during interactive tui startup")
	}
	if app.Verbose {
		t.Fatal("expected quiet mode to remain disabled after interactive startup")
	}
}

func TestRunFixInteractiveWithMutedLogsRestoresVerboseOnError(t *testing.T) {
	t.Parallel()

	app := &App{
		Verbose: true,
		IsInteractiveTerminal: func() bool {
			return true
		},
	}

	boom := errors.New("boom")
	var verboseDuringCall bool
	app.runFixInteractiveFn = func(_ []string, _ bool) (int, error) {
		verboseDuringCall = app.Verbose
		return 2, boom
	}

	code, err := app.runFix(FixOptions{})
	if !errors.Is(err, boom) {
		t.Fatalf("runFix error = %v, want %v", err, boom)
	}
	if code != 2 {
		t.Fatalf("runFix code = %d, want 2", code)
	}
	if verboseDuringCall {
		t.Fatal("expected verbose logging to be disabled during interactive tui startup")
	}
	if !app.Verbose {
		t.Fatal("expected verbose setting to be restored after interactive startup error")
	}
}
