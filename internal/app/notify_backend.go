package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"bb-project/internal/domain"
)

const (
	notifyBackendStdout    = "stdout"
	notifyBackendOSAScript = "osascript"
	notifyBackendEnvVar    = "BB_NOTIFY_BACKEND"
)

type notifyMessage struct {
	Repo        domain.MachineRepoRecord
	Fingerprint string
}

type notifySender interface {
	Send(msg notifyMessage) error
}

type stdoutNotifySender struct {
	out io.Writer
}

func (s *stdoutNotifySender) Send(msg notifyMessage) error {
	_, err := fmt.Fprintf(s.out, "notify %s: %s\n", msg.Repo.Name, msg.Fingerprint)
	return err
}

type osascriptNotifySender struct {
	runCommand func(name string, args ...string) (string, error)
}

func (s *osascriptNotifySender) Send(msg notifyMessage) error {
	title := "bb sync"
	subtitle := msg.Repo.Name
	body := fmt.Sprintf("Unsyncable: %s", msg.Fingerprint)
	script := fmt.Sprintf("display notification %s with title %s subtitle %s", applescriptQuote(body), applescriptQuote(title), applescriptQuote(subtitle))
	out, err := s.runCommand("osascript", "-e", script)
	if err != nil {
		return fmt.Errorf("osascript notify failed: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func defaultRunCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func newNotifySender(backend string, out io.Writer, runCommand func(name string, args ...string) (string, error)) (notifySender, error) {
	switch backend {
	case notifyBackendStdout:
		return &stdoutNotifySender{out: out}, nil
	case notifyBackendOSAScript:
		if runCommand == nil {
			runCommand = defaultRunCommand
		}
		return &osascriptNotifySender{runCommand: runCommand}, nil
	default:
		return nil, fmt.Errorf("invalid notify backend %q (supported: %s, %s)", backend, notifyBackendStdout, notifyBackendOSAScript)
	}
}

func (a *App) resolveNotifyBackend(override string) (string, error) {
	return a.resolveNotifyBackendWithDefault(override, notifyBackendStdout)
}

func (a *App) resolveNotifyBackendWithDefault(override string, defaultBackend string) (string, error) {
	backend := strings.ToLower(strings.TrimSpace(override))
	if backend == "" {
		getenv := os.Getenv
		if a.Getenv != nil {
			getenv = a.Getenv
		}
		backend = strings.ToLower(strings.TrimSpace(getenv(notifyBackendEnvVar)))
	}
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(defaultBackend))
	}
	switch backend {
	case notifyBackendStdout, notifyBackendOSAScript:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid notify backend %q", backend)
	}
}

func applescriptQuote(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`"`, `\"`,
		"\n", "\\n",
		"\r", "",
	)
	return `"` + replacer.Replace(value) + `"`
}

func notifyFailureCacheKey(backend string, rec domain.MachineRepoRecord) string {
	return backend + "|" + notifyCacheKey(rec)
}
