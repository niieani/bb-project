package testharness

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	buildPath string
	buildErr  error
)

type Harness struct {
	t           *testing.T
	Root        string
	RemotesRoot string
	SharedRoot  string
	BinaryPath  string
	Machines    map[string]*Machine
}

type Machine struct {
	t       *testing.T
	Harness *Harness
	ID      string
	Home    string
}

func NewHarness(t *testing.T) *Harness {
	t.Helper()

	root := t.TempDir()
	remotes := filepath.Join(root, "remotes")
	shared := filepath.Join(root, "shared-state")
	mustMkdirAll(t, remotes)
	mustMkdirAll(t, shared)

	bin := buildBinary(t)
	return &Harness{t: t, Root: root, RemotesRoot: remotes, SharedRoot: shared, BinaryPath: bin, Machines: map[string]*Machine{}}
}

func buildBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		path := filepath.Join(os.TempDir(), fmt.Sprintf("bb-test-%d", time.Now().UnixNano()))
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", path, "./cmd/bb")
		cmd.Dir = repoRootFromWD(t)
		cmd.Env = append(os.Environ(), "GOCACHE=/tmp/go-cache", "GOMODCACHE=/tmp/go-mod")
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("build bb: %w: %s", err, string(out))
			return
		}
		buildPath = path
	})

	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildPath
}

func repoRootFromWD(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatalf("could not find go.mod from %q", wd)
		}
		wd = parent
	}
}

func (h *Harness) AddMachine(machineID string, catalogs map[string]string, defaultCatalog string) *Machine {
	h.t.Helper()

	home := filepath.Join(h.Root, "machines", machineID, "home")
	mustMkdirAll(h.t, home)
	m := &Machine{t: h.t, Harness: h, ID: machineID, Home: home}
	h.Machines[machineID] = m

	for _, root := range catalogs {
		mustMkdirAll(h.t, root)
	}

	machineYAML := m.ConfigMachinePath()
	mustMkdirAll(h.t, filepath.Dir(machineYAML))
	var b strings.Builder
	b.WriteString("version: 1\n")
	b.WriteString("machine_id: ")
	b.WriteString(machineID)
	b.WriteString("\n")
	b.WriteString("hostname: ")
	b.WriteString(machineID)
	b.WriteString("\n")
	b.WriteString("default_catalog: ")
	b.WriteString(defaultCatalog)
	b.WriteString("\n")
	b.WriteString("catalogs:\n")

	names := make([]string, 0, len(catalogs))
	for n := range catalogs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b.WriteString("  - name: ")
		b.WriteString(n)
		b.WriteString("\n")
		b.WriteString("    root: ")
		b.WriteString(catalogs[n])
		b.WriteString("\n")
	}
	if err := os.WriteFile(machineYAML, []byte(b.String()), 0o644); err != nil {
		h.t.Fatalf("write machine yaml: %v", err)
	}

	configYAML := m.ConfigPath()
	mustMkdirAll(h.t, filepath.Dir(configYAML))
	if err := os.WriteFile(configYAML, []byte(defaultConfigYAML()), 0o644); err != nil {
		h.t.Fatalf("write config yaml: %v", err)
	}

	return m
}

func defaultConfigYAML() string {
	return strings.TrimSpace(`
version: 1
state_transport:
  mode: external
github:
  owner: you
  default_visibility: private
  remote_protocol: ssh
sync:
  auto_discover: true
  include_untracked_as_dirty: true
  default_auto_push_private: true
  default_auto_push_public: false
  fetch_prune: true
  pull_ff_only: true
notify:
  enabled: true
  dedupe: true
  throttle_minutes: 60
`) + "\n"
}

func (m *Machine) ConfigRoot() string {
	return filepath.Join(m.Home, ".config", "bb-project")
}

func (m *Machine) ConfigPath() string {
	return filepath.Join(m.ConfigRoot(), "config.yaml")
}

func (m *Machine) ConfigMachinePath() string {
	return filepath.Join(m.ConfigRoot(), "machines", m.ID+".yaml")
}

func (m *Machine) ReposDir() string {
	return filepath.Join(m.ConfigRoot(), "repos")
}

func (m *Machine) LocalStateRoot() string {
	return filepath.Join(m.Home, ".local", "state", "bb-project")
}

func (m *Machine) RunBB(now time.Time, args ...string) (string, error) {
	m.t.Helper()
	return m.RunBBAt(now, m.Home, args...)
}

func (m *Machine) RunBBAt(now time.Time, dir string, args ...string) (string, error) {
	m.t.Helper()

	cmd := exec.Command(m.Harness.BinaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+m.Home,
		"BB_MACHINE_ID="+m.ID,
		"BB_TEST_REMOTE_ROOT="+m.Harness.RemotesRoot,
		"BB_NOW="+now.UTC().Format(time.RFC3339),
		"GOCACHE=/tmp/go-cache",
		"GOMODCACHE=/tmp/go-mod",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=bb-test",
		"GIT_AUTHOR_EMAIL=bb-test@example.com",
		"GIT_COMMITTER_NAME=bb-test",
		"GIT_COMMITTER_EMAIL=bb-test@example.com",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	return out, err
}

func (m *Machine) RunGit(now time.Time, dir string, args ...string) (string, error) {
	m.t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+m.Home,
		"BB_MACHINE_ID="+m.ID,
		"BB_NOW="+now.UTC().Format(time.RFC3339),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=bb-test",
		"GIT_AUTHOR_EMAIL=bb-test@example.com",
		"GIT_COMMITTER_NAME=bb-test",
		"GIT_COMMITTER_EMAIL=bb-test@example.com",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (m *Machine) MustRunGit(now time.Time, dir string, args ...string) string {
	m.t.Helper()
	out, err := m.RunGit(now, dir, args...)
	if err != nil {
		m.t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
	return out
}

func (m *Machine) MustWriteFile(path, contents string) {
	m.t.Helper()
	mustMkdirAll(m.t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		m.t.Fatalf("write file %s: %v", path, err)
	}
}

func (m *Machine) MustReadFile(path string) string {
	m.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		m.t.Fatalf("read file %s: %v", path, err)
	}
	return string(b)
}

func (h *Harness) ExternalSync(order ...string) {
	h.t.Helper()

	if len(order) == 0 {
		for id := range h.Machines {
			order = append(order, id)
		}
		sort.Strings(order)
	}

	sharedConfig := filepath.Join(h.SharedRoot, ".config", "bb-project")
	_ = os.RemoveAll(sharedConfig)
	mustMkdirAll(h.t, sharedConfig)

	for _, id := range order {
		m := h.Machines[id]
		if m == nil {
			h.t.Fatalf("unknown machine %q", id)
		}
		// config.yaml is shared and low-churn; last writer wins.
		copyFileIfExists(h.t, filepath.Join(m.ConfigRoot(), "config.yaml"), filepath.Join(sharedConfig, "config.yaml"))
		// repos metadata is shared.
		copyTree(h.t, filepath.Join(m.ConfigRoot(), "repos"), filepath.Join(sharedConfig, "repos"))
		// each machine writes only its own machine file.
		copyFileIfExists(
			h.t,
			filepath.Join(m.ConfigRoot(), "machines", m.ID+".yaml"),
			filepath.Join(sharedConfig, "machines", m.ID+".yaml"),
		)
	}

	for _, m := range h.Machines {
		_ = os.RemoveAll(m.ConfigRoot())
		copyTree(h.t, sharedConfig, m.ConfigRoot())
	}
}

func copyFileIfExists(t *testing.T, src, dst string) {
	t.Helper()
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("stat %s: %v", src, err)
	}
	copyFile(t, src, dst)
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("readdir %s: %v", src, err)
	}
	mustMkdirAll(t, dst)
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyTree(t, s, d)
			continue
		}
		copyFile(t, s, d)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(dst))
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src %s: %v", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create dst %s: %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close dst %s: %v", dst, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
