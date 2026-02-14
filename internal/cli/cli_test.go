package cli

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"bb-project/internal/app"
	"bb-project/internal/state"
)

type fakeApp struct {
	verbose bool

	initOpts   app.InitOptions
	scanOpts   app.ScanOptions
	syncOpts   app.SyncOptions
	fixOpts    app.FixOptions
	statusJSON bool
	statusIncl []string
	doctorIncl []string
	ensureIncl []string

	repoPolicySelector  string
	repoPolicyAutoPush  bool
	repoRemoteSelector  string
	repoPreferredRemote string

	catalogAddName string
	catalogAddRoot string
	catalogRMName  string
	catalogDefName string

	initErr         error
	scanCode        int
	scanErr         error
	syncCode        int
	syncErr         error
	fixCode         int
	fixErr          error
	statusCode      int
	statusErr       error
	doctorCode      int
	doctorErr       error
	ensureCode      int
	ensureErr       error
	repoPolicyCode  int
	repoPolicyErr   error
	repoRemoteCode  int
	repoRemoteErr   error
	catalogAddCode  int
	catalogAddErr   error
	catalogRMCode   int
	catalogRMErr    error
	catalogDefCode  int
	catalogDefErr   error
	catalogListCode int
	catalogListErr  error
	configErr       error
}

func (f *fakeApp) SetVerbose(verbose bool) {
	f.verbose = verbose
}

func (f *fakeApp) RunInit(opts app.InitOptions) error {
	f.initOpts = opts
	return f.initErr
}

func (f *fakeApp) RunScan(opts app.ScanOptions) (int, error) {
	f.scanOpts = opts
	return f.scanCode, f.scanErr
}

func (f *fakeApp) RunSync(opts app.SyncOptions) (int, error) {
	f.syncOpts = opts
	return f.syncCode, f.syncErr
}

func (f *fakeApp) RunFix(opts app.FixOptions) (int, error) {
	f.fixOpts = opts
	return f.fixCode, f.fixErr
}

func (f *fakeApp) RunStatus(jsonOut bool, include []string) (int, error) {
	f.statusJSON = jsonOut
	f.statusIncl = append([]string(nil), include...)
	return f.statusCode, f.statusErr
}

func (f *fakeApp) RunDoctor(include []string) (int, error) {
	f.doctorIncl = append([]string(nil), include...)
	return f.doctorCode, f.doctorErr
}

func (f *fakeApp) RunEnsure(include []string) (int, error) {
	f.ensureIncl = append([]string(nil), include...)
	return f.ensureCode, f.ensureErr
}

func (f *fakeApp) RunRepoPolicy(repoSelector string, autoPush bool) (int, error) {
	f.repoPolicySelector = repoSelector
	f.repoPolicyAutoPush = autoPush
	return f.repoPolicyCode, f.repoPolicyErr
}

func (f *fakeApp) RunRepoPreferredRemote(repoSelector string, preferredRemote string) (int, error) {
	f.repoRemoteSelector = repoSelector
	f.repoPreferredRemote = preferredRemote
	return f.repoRemoteCode, f.repoRemoteErr
}

func (f *fakeApp) RunCatalogAdd(name, root string) (int, error) {
	f.catalogAddName = name
	f.catalogAddRoot = root
	return f.catalogAddCode, f.catalogAddErr
}

func (f *fakeApp) RunCatalogRM(name string) (int, error) {
	f.catalogRMName = name
	return f.catalogRMCode, f.catalogRMErr
}

func (f *fakeApp) RunCatalogDefault(name string) (int, error) {
	f.catalogDefName = name
	return f.catalogDefCode, f.catalogDefErr
}

func (f *fakeApp) RunCatalogList() (int, error) {
	return f.catalogListCode, f.catalogListErr
}

func (f *fakeApp) RunConfig() error {
	return f.configErr
}

func TestRunHelpDoesNotCreateApp(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, stdout, stderr, calls, _ := runCLI(t, fake, []string{"help"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if calls != 0 {
		t.Fatalf("app factory calls = %d, want 0", calls)
	}
	mustContain(t, stdout, "Usage:")
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunRequiresCommand(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, _, stderr, calls, _ := runCLI(t, fake, nil)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if calls != 0 {
		t.Fatalf("app factory calls = %d, want 0", calls)
	}
	mustContain(t, stderr, "a command is required")
}

func TestRunUnknownCommandReturns2(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, _, stderr, calls, _ := runCLI(t, fake, []string{"nope"})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if calls != 0 {
		t.Fatalf("app factory calls = %d, want 0", calls)
	}
	mustContain(t, stderr, "unknown command")
}

func TestRunQuietFlagApplied(t *testing.T) {
	t.Parallel()

	t.Run("quiet true", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, paths := runCLI(t, fake, []string{"scan", "--quiet"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if calls != 1 {
			t.Fatalf("app factory calls = %d, want 1", calls)
		}
		if fake.verbose {
			t.Fatal("expected verbose=false when --quiet is set")
		}
		if got := paths.ConfigRoot(); got != "/tmp/test-home/.config/bb-project" {
			t.Fatalf("config root = %q, want %q", got, "/tmp/test-home/.config/bb-project")
		}
	})

	t.Run("quiet false", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"scan"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if calls != 1 {
			t.Fatalf("app factory calls = %d, want 1", calls)
		}
		if !fake.verbose {
			t.Fatal("expected verbose=true by default")
		}
	})
}

func TestRunInitForwardsOptions(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, _, stderr, _, _ := runCLI(t, fake, []string{"init", "proj", "--catalog", "software", "--public", "--push", "--https"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	want := app.InitOptions{Project: "proj", Catalog: "software", Public: true, Push: true, HTTPS: true}
	if fake.initOpts != want {
		t.Fatalf("init opts = %#v, want %#v", fake.initOpts, want)
	}
}

func TestRunInitRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, _, stderr, calls, _ := runCLI(t, fake, []string{"init", "a", "b"})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if calls != 0 {
		t.Fatalf("app factory calls = %d, want 0", calls)
	}
	mustContain(t, stderr, "accepts at most 1 arg")
}

func TestRunScanAndSyncForwardOptions(t *testing.T) {
	t.Parallel()

	t.Run("scan include catalogs", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scan", "--include-catalog", "software", "--include-catalog", "references"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		mustEqualSlices(t, fake.scanOpts.IncludeCatalogs, []string{"software", "references"})
	})

	t.Run("sync flags", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"sync", "--include-catalog", "software", "--push", "--notify", "--dry-run"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if !fake.syncOpts.Push || !fake.syncOpts.Notify || !fake.syncOpts.DryRun {
			t.Fatalf("sync flags not forwarded: %#v", fake.syncOpts)
		}
		mustEqualSlices(t, fake.syncOpts.IncludeCatalogs, []string{"software"})
	})
}

func TestRunStatusDoctorEnsureForwardOptions(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, _, stderr, _, _ := runCLI(t, fake, []string{"status", "--json", "--include-catalog", "software", "--include-catalog", "references"})
	if code != 0 {
		t.Fatalf("status exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !fake.statusJSON {
		t.Fatal("expected json output flag to be forwarded")
	}
	mustEqualSlices(t, fake.statusIncl, []string{"software", "references"})

	fake = &fakeApp{}
	code, _, stderr, _, _ = runCLI(t, fake, []string{"doctor", "--include-catalog", "software"})
	if code != 0 {
		t.Fatalf("doctor exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	mustEqualSlices(t, fake.doctorIncl, []string{"software"})

	fake = &fakeApp{}
	code, _, stderr, _, _ = runCLI(t, fake, []string{"ensure", "--include-catalog", "software"})
	if code != 0 {
		t.Fatalf("ensure exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	mustEqualSlices(t, fake.ensureIncl, []string{"software"})
}

func TestRunFixForwardsOptions(t *testing.T) {
	t.Parallel()

	t.Run("interactive mode with catalogs", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"fix", "--include-catalog", "software", "--include-catalog", "references"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.fixOpts.Project != "" || fake.fixOpts.Action != "" {
			t.Fatalf("unexpected project/action: %#v", fake.fixOpts)
		}
		if fake.fixOpts.NoRefresh {
			t.Fatalf("no-refresh = %t, want false", fake.fixOpts.NoRefresh)
		}
		mustEqualSlices(t, fake.fixOpts.IncludeCatalogs, []string{"software", "references"})
	})

	t.Run("project lookup mode", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"fix", "api"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.fixOpts.Project != "api" {
			t.Fatalf("project = %q, want api", fake.fixOpts.Project)
		}
		if fake.fixOpts.Action != "" {
			t.Fatalf("action = %q, want empty", fake.fixOpts.Action)
		}
	})

	t.Run("apply mode with auto message", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"fix", "api", "stage-commit-push", "--message=auto"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.fixOpts.Project != "api" || fake.fixOpts.Action != "stage-commit-push" {
			t.Fatalf("unexpected fix opts: %#v", fake.fixOpts)
		}
		if fake.fixOpts.CommitMessage != "auto" {
			t.Fatalf("commit message = %q, want auto", fake.fixOpts.CommitMessage)
		}
	})

	t.Run("forwards no-refresh", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"fix", "--no-refresh"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if !fake.fixOpts.NoRefresh {
			t.Fatalf("no-refresh = %t, want true", fake.fixOpts.NoRefresh)
		}
	})
}

func TestRunRepoPolicyValidationAndForwarding(t *testing.T) {
	t.Parallel()

	t.Run("requires flag", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"repo", "policy", "demo"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "required")
		mustContain(t, stderr, "auto-push")
	})

	t.Run("invalid bool", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"repo", "policy", "demo", "--auto-push=not-bool"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 1 {
			t.Fatalf("app factory calls = %d, want 1", calls)
		}
		mustContain(t, stderr, "invalid --auto-push")
	})

	t.Run("forwards values", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"repo", "policy", "demo", "--auto-push=false"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.repoPolicySelector != "demo" || fake.repoPolicyAutoPush {
			t.Fatalf("repo policy forwarding mismatch: selector=%q autoPush=%v", fake.repoPolicySelector, fake.repoPolicyAutoPush)
		}
	})

	t.Run("remote requires flag", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"repo", "remote", "demo"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "required")
		mustContain(t, stderr, "preferred-remote")
	})

	t.Run("remote forwards values", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"repo", "remote", "demo", "--preferred-remote=upstream"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.repoRemoteSelector != "demo" {
			t.Fatalf("repo selector = %q, want demo", fake.repoRemoteSelector)
		}
		if fake.repoPreferredRemote != "upstream" {
			t.Fatalf("preferred remote = %q, want upstream", fake.repoPreferredRemote)
		}
	})
}

func TestRunCatalogAndConfigCommands(t *testing.T) {
	t.Parallel()

	t.Run("catalog add", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"catalog", "add", "software", "/tmp/software"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.catalogAddName != "software" || fake.catalogAddRoot != "/tmp/software" {
			t.Fatalf("catalog add forwarding mismatch: name=%q root=%q", fake.catalogAddName, fake.catalogAddRoot)
		}
	})

	t.Run("config rejects args", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"config", "extra"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "unknown command")
	})
}

func TestRunExitCodePropagation(t *testing.T) {
	t.Parallel()

	t.Run("unsyncable code 1", func(t *testing.T) {
		fake := &fakeApp{scanCode: 1}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scan"})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("app error with explicit code", func(t *testing.T) {
		fake := &fakeApp{scanCode: 2, scanErr: errors.New("scan failed")}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scan"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		mustContain(t, stderr, "scan failed")
	})

	t.Run("app error defaults to code 2", func(t *testing.T) {
		fake := &fakeApp{scanCode: 0, scanErr: errors.New("boom")}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scan"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		mustContain(t, stderr, "boom")
	})
}

func TestRunCompletionCommand(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	code, stdout, stderr, calls, _ := runCLI(t, fake, []string{"completion", "bash"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if calls != 0 {
		t.Fatalf("app factory calls = %d, want 0", calls)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	mustContain(t, stdout, "__start_bb")
}

func TestRunHomeResolveFailure(t *testing.T) {
	t.Parallel()

	fake := &fakeApp{}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithDeps([]string{"scan"}, &stdout, &stderr, runDeps{
		userHomeDir: func() (string, error) {
			return "", errors.New("no home")
		},
		newApp: func(paths state.Paths, stdout io.Writer, stderr io.Writer) appRunner {
			return fake
		},
	})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	mustContain(t, stderr.String(), "resolve home")
}

func runCLI(t *testing.T, fake *fakeApp, args []string) (int, string, string, int, state.Paths) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	factoryCalls := 0
	var createdPaths state.Paths

	code := runWithDeps(args, &stdout, &stderr, runDeps{
		userHomeDir: func() (string, error) {
			return "/tmp/test-home", nil
		},
		newApp: func(paths state.Paths, stdout io.Writer, stderr io.Writer) appRunner {
			factoryCalls++
			createdPaths = paths
			return fake
		},
	})

	return code, stdout.String(), stderr.String(), factoryCalls, createdPaths
}

func mustContain(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}

func mustEqualSlices(t *testing.T, got []string, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("slice = %v, want %v", got, want)
	}
}
