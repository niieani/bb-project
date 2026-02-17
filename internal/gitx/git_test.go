package gitx

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

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

func TestLooksLikeSyncConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{name: "merge conflict", msg: "CONFLICT (content): Merge conflict in file.txt", want: true},
		{name: "rebase could not apply", msg: "error: could not apply abc123", want: true},
		{name: "network timeout", msg: "fatal: unable to access remote: timeout", want: false},
		{name: "empty", msg: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeSyncConflict(tt.msg); got != tt.want {
				t.Fatalf("looksLikeSyncConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLooksLikePushRejectedNonFastForward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "non-fast-forward rejection",
			msg: `error: failed to push some refs to '/tmp/demo.git'
hint: Updates were rejected because the tip of your current branch is behind
hint: its remote counterpart.`,
			want: true,
		},
		{name: "explicit non-fast-forward", msg: "non-fast-forward", want: true},
		{name: "access denied", msg: "permission denied", want: false},
		{name: "empty", msg: "", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikePushRejectedNonFastForward(tt.msg); got != tt.want {
				t.Fatalf("looksLikePushRejectedNonFastForward() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitCommandEnvDisablesInteractivePrompts(t *testing.T) {
	t.Parallel()

	env := gitCommandEnv([]string{
		"PATH=/usr/bin",
		"GIT_TERMINAL_PROMPT=1",
		"GCM_INTERACTIVE=always",
		"GIT_ASKPASS=/tmp/askpass",
		"SSH_ASKPASS=/tmp/ssh-askpass",
		"SSH_ASKPASS_REQUIRE=force",
	}, GitIOModeNonInteractive)

	values := map[string]string{}
	for _, entry := range env {
		key, value, ok := splitEnvEntry(entry)
		if !ok {
			continue
		}
		values[key] = value
	}

	if got := values["GIT_TERMINAL_PROMPT"]; got != "0" {
		t.Fatalf("GIT_TERMINAL_PROMPT = %q, want %q", got, "0")
	}
	if got := values["GCM_INTERACTIVE"]; got != "never" {
		t.Fatalf("GCM_INTERACTIVE = %q, want %q", got, "never")
	}
	if got := values["GIT_ASKPASS"]; got != "" {
		t.Fatalf("GIT_ASKPASS = %q, want empty", got)
	}
	if got := values["SSH_ASKPASS"]; got != "" {
		t.Fatalf("SSH_ASKPASS = %q, want empty", got)
	}
	if got := values["SSH_ASKPASS_REQUIRE"]; got != "never" {
		t.Fatalf("SSH_ASKPASS_REQUIRE = %q, want %q", got, "never")
	}
	if got := values["GIT_CONFIG_GLOBAL"]; got != os.DevNull {
		t.Fatalf("GIT_CONFIG_GLOBAL = %q, want %q", got, os.DevNull)
	}
}

func TestGitCommandEnvAllowsInteractivePromptsInAttachedMode(t *testing.T) {
	t.Parallel()

	env := gitCommandEnv([]string{
		"PATH=/usr/bin",
		"GIT_TERMINAL_PROMPT=1",
		"GCM_INTERACTIVE=always",
		"GIT_ASKPASS=/tmp/askpass",
		"SSH_ASKPASS=/tmp/ssh-askpass",
		"SSH_ASKPASS_REQUIRE=force",
	}, GitIOModeAttached)

	values := map[string]string{}
	for _, entry := range env {
		key, value, ok := splitEnvEntry(entry)
		if !ok {
			continue
		}
		values[key] = value
	}

	if got := values["GIT_TERMINAL_PROMPT"]; got != "1" {
		t.Fatalf("GIT_TERMINAL_PROMPT = %q, want %q", got, "1")
	}
	if got := values["GCM_INTERACTIVE"]; got != "always" {
		t.Fatalf("GCM_INTERACTIVE = %q, want %q", got, "always")
	}
	if got := values["GIT_ASKPASS"]; got != "/tmp/askpass" {
		t.Fatalf("GIT_ASKPASS = %q, want /tmp/askpass", got)
	}
	if got := values["SSH_ASKPASS"]; got != "/tmp/ssh-askpass" {
		t.Fatalf("SSH_ASKPASS = %q, want /tmp/ssh-askpass", got)
	}
	if got := values["SSH_ASKPASS_REQUIRE"]; got != "force" {
		t.Fatalf("SSH_ASKPASS_REQUIRE = %q, want force", got)
	}
}

func TestRunnerAttachedModePassesThroughConfiguredStdio(t *testing.T) {
	t.Parallel()

	var passthroughOut bytes.Buffer
	var passthroughErr bytes.Buffer
	r := Runner{
		IOMode: GitIOModeAttached,
		Stdin:  strings.NewReader("hello\n"),
		Stdout: &passthroughOut,
		Stderr: &passthroughErr,
	}

	result, err := r.runWithEnvStreaming(
		"",
		nil,
		nil,
		nil,
		"bash",
		"-lc",
		`read line || exit 42; echo "out:$line"; echo "err:$line" >&2`,
	)
	if err != nil {
		t.Fatalf("runWithEnvStreaming failed: %v", err)
	}
	if !strings.Contains(result.Stdout, "out:hello") {
		t.Fatalf("result stdout = %q, want out:hello", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "err:hello") {
		t.Fatalf("result stderr = %q, want err:hello", result.Stderr)
	}
	if !strings.Contains(passthroughOut.String(), "out:hello") {
		t.Fatalf("passthrough stdout = %q, want out:hello", passthroughOut.String())
	}
	if !strings.Contains(passthroughErr.String(), "err:hello") {
		t.Fatalf("passthrough stderr = %q, want err:hello", passthroughErr.String())
	}
}

func splitEnvEntry(entry string) (string, string, bool) {
	for i := 0; i < len(entry); i++ {
		if entry[i] == '=' {
			return entry[:i], entry[i+1:], true
		}
	}
	return "", "", false
}

func TestRenameCurrentBranch(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	r := Runner{}
	if err := r.InitRepo(repoPath); err != nil {
		t.Fatalf("init repo failed: %v", err)
	}
	if err := os.WriteFile(repoPath+"/README.md", []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := r.AddAll(repoPath); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := r.Commit(repoPath, "init"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	if err := r.RenameCurrentBranch(repoPath, "feature/rename-check"); err != nil {
		t.Fatalf("rename current branch failed: %v", err)
	}
	branch, err := r.CurrentBranch(repoPath)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "feature/rename-check" {
		t.Fatalf("current branch = %q, want %q", branch, "feature/rename-check")
	}
}
