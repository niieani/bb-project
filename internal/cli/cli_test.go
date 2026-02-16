package cli

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"bb-project/internal/app"
	"bb-project/internal/domain"
	"bb-project/internal/state"
)

type fakeApp struct {
	verbose bool

	initOpts   app.InitOptions
	scanOpts   app.ScanOptions
	syncOpts   app.SyncOptions
	fixOpts    app.FixOptions
	cloneOpts  app.CloneOptions
	linkOpts   app.LinkOptions
	statusJSON bool
	statusIncl []string
	doctorIncl []string
	ensureIncl []string

	repoPolicySelector  string
	repoPolicyAutoPush  domain.AutoPushMode
	repoRemoteSelector  string
	repoPreferredRemote string
	repoAccessSelector  string
	repoAccessValue     string
	repoRefreshSelector string

	catalogAddName string
	catalogAddRoot string
	catalogRMName  string
	catalogDefName string

	schedulerInstallOpts  app.SchedulerInstallOptions
	schedulerInstallCalls int
	schedulerStatusCalls  int
	schedulerRemoveCalls  int

	initErr         error
	scanCode        int
	scanErr         error
	syncCode        int
	syncErr         error
	fixCode         int
	fixErr          error
	cloneCode       int
	cloneErr        error
	linkCode        int
	linkErr         error
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
	repoAccessCode  int
	repoAccessErr   error
	repoRefreshCode int
	repoRefreshErr  error
	catalogAddCode  int
	catalogAddErr   error
	catalogRMCode   int
	catalogRMErr    error
	catalogDefCode  int
	catalogDefErr   error
	catalogListCode int
	catalogListErr  error
	configErr       error

	schedulerInstallCode int
	schedulerInstallErr  error
	schedulerStatusCode  int
	schedulerStatusErr   error
	schedulerRemoveCode  int
	schedulerRemoveErr   error
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

func (f *fakeApp) RunClone(opts app.CloneOptions) (int, error) {
	f.cloneOpts = opts
	return f.cloneCode, f.cloneErr
}

func (f *fakeApp) RunLink(opts app.LinkOptions) (int, error) {
	f.linkOpts = opts
	return f.linkCode, f.linkErr
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

func (f *fakeApp) RunRepoPolicy(repoSelector string, autoPushMode domain.AutoPushMode) (int, error) {
	f.repoPolicySelector = repoSelector
	f.repoPolicyAutoPush = autoPushMode
	return f.repoPolicyCode, f.repoPolicyErr
}

func (f *fakeApp) RunRepoPreferredRemote(repoSelector string, preferredRemote string) (int, error) {
	f.repoRemoteSelector = repoSelector
	f.repoPreferredRemote = preferredRemote
	return f.repoRemoteCode, f.repoRemoteErr
}

func (f *fakeApp) RunRepoPushAccessSet(repoSelector string, pushAccess string) (int, error) {
	f.repoAccessSelector = repoSelector
	f.repoAccessValue = pushAccess
	return f.repoAccessCode, f.repoAccessErr
}

func (f *fakeApp) RunRepoPushAccessRefresh(repoSelector string) (int, error) {
	f.repoRefreshSelector = repoSelector
	return f.repoRefreshCode, f.repoRefreshErr
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

func (f *fakeApp) RunSchedulerInstall(opts app.SchedulerInstallOptions) (int, error) {
	f.schedulerInstallCalls++
	f.schedulerInstallOpts = opts
	return f.schedulerInstallCode, f.schedulerInstallErr
}

func (f *fakeApp) RunSchedulerStatus() (int, error) {
	f.schedulerStatusCalls++
	return f.schedulerStatusCode, f.schedulerStatusErr
}

func (f *fakeApp) RunSchedulerRemove() (int, error) {
	f.schedulerRemoveCalls++
	return f.schedulerRemoveCode, f.schedulerRemoveErr
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
		code, _, stderr, _, _ := runCLI(t, fake, []string{"sync", "--include-catalog", "software", "--push", "--notify", "--dry-run", "--notify-backend", "osascript"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if !fake.syncOpts.Push || !fake.syncOpts.Notify || !fake.syncOpts.DryRun {
			t.Fatalf("sync flags not forwarded: %#v", fake.syncOpts)
		}
		if fake.syncOpts.NotifyBackend != "osascript" {
			t.Fatalf("notify backend = %q, want %q", fake.syncOpts.NotifyBackend, "osascript")
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
		if fake.fixOpts.SyncStrategy != app.FixSyncStrategyRebase {
			t.Fatalf("sync strategy = %q, want %q", fake.fixOpts.SyncStrategy, app.FixSyncStrategyRebase)
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

	t.Run("forwards explicit sync strategy", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"fix", "api", "sync-with-upstream", "--sync-strategy=merge"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.fixOpts.SyncStrategy != app.FixSyncStrategyMerge {
			t.Fatalf("sync strategy = %q, want %q", fake.fixOpts.SyncStrategy, app.FixSyncStrategyMerge)
		}
	})

	t.Run("forwards publish-branch and return-sync options", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{
			"fix", "api", "publish-new-branch",
			"--publish-branch=feature/safe-publish",
			"--return-to-original-sync",
			"--message=auto",
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.fixOpts.PublishBranch != "feature/safe-publish" {
			t.Fatalf("publish branch = %q, want %q", fake.fixOpts.PublishBranch, "feature/safe-publish")
		}
		if !fake.fixOpts.ReturnToOriginalBranchAndSync {
			t.Fatal("expected return-to-original-sync option to be forwarded")
		}
	})

	t.Run("invalid sync strategy returns usage error", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"fix", "--sync-strategy=invalid"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "invalid --sync-strategy")
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

	t.Run("invalid mode", func(t *testing.T) {
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
		if fake.repoPolicySelector != "demo" || fake.repoPolicyAutoPush != domain.AutoPushModeDisabled {
			t.Fatalf("repo policy forwarding mismatch: selector=%q autoPush=%q", fake.repoPolicySelector, fake.repoPolicyAutoPush)
		}
	})

	t.Run("forwards include-default mode", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"repo", "policy", "demo", "--auto-push=include-default-branch"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.repoPolicySelector != "demo" || fake.repoPolicyAutoPush != domain.AutoPushModeIncludeDefaultBranch {
			t.Fatalf("repo policy forwarding mismatch: selector=%q autoPush=%q", fake.repoPolicySelector, fake.repoPolicyAutoPush)
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

	t.Run("access-set requires flag", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"repo", "access-set", "demo"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "required")
		mustContain(t, stderr, "push-access")
	})

	t.Run("access-set forwards values", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"repo", "access-set", "demo", "--push-access=read_only"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.repoAccessSelector != "demo" || fake.repoAccessValue != "read_only" {
			t.Fatalf("repo access forwarding mismatch: selector=%q value=%q", fake.repoAccessSelector, fake.repoAccessValue)
		}
	})

	t.Run("access-refresh forwards values", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"repo", "access-refresh", "demo"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.repoRefreshSelector != "demo" {
			t.Fatalf("repo refresh selector = %q, want demo", fake.repoRefreshSelector)
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

	t.Run("scheduler install forwards backend", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scheduler", "install", "--notify-backend", "osascript"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.schedulerInstallCalls != 1 {
			t.Fatalf("install calls = %d, want 1", fake.schedulerInstallCalls)
		}
		if fake.schedulerInstallOpts.NotifyBackend != "osascript" {
			t.Fatalf("notify backend = %q, want %q", fake.schedulerInstallOpts.NotifyBackend, "osascript")
		}
	})

	t.Run("scheduler status", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scheduler", "status"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.schedulerStatusCalls != 1 {
			t.Fatalf("status calls = %d, want 1", fake.schedulerStatusCalls)
		}
	})

	t.Run("scheduler remove", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{"scheduler", "remove"})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.schedulerRemoveCalls != 1 {
			t.Fatalf("remove calls = %d, want 1", fake.schedulerRemoveCalls)
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

func TestRunCloneAndLinkForwardOptions(t *testing.T) {
	t.Parallel()

	t.Run("clone missing repo shows command hint", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"clone"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "bb clone")
		mustContain(t, stderr, "accepts 1 arg(s), received 0")
	})

	t.Run("clone forwards flags", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{
			"clone",
			"https://github.com/openai/codex",
			"--catalog", "references",
			"--as", "openai/codex",
			"--shallow",
			"--filter", "blob:none",
			"--only", "README.md",
			"--only", "docs",
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.cloneOpts.Repo != "https://github.com/openai/codex" {
			t.Fatalf("repo = %q, want %q", fake.cloneOpts.Repo, "https://github.com/openai/codex")
		}
		if fake.cloneOpts.Catalog != "references" {
			t.Fatalf("catalog = %q, want %q", fake.cloneOpts.Catalog, "references")
		}
		if fake.cloneOpts.As != "openai/codex" {
			t.Fatalf("as = %q, want %q", fake.cloneOpts.As, "openai/codex")
		}
		if !fake.cloneOpts.ShallowSet || !fake.cloneOpts.Shallow {
			t.Fatalf("shallow forwarding mismatch: %#v", fake.cloneOpts)
		}
		if !fake.cloneOpts.FilterSet || fake.cloneOpts.Filter != "blob:none" {
			t.Fatalf("filter forwarding mismatch: %#v", fake.cloneOpts)
		}
		mustEqualSlices(t, fake.cloneOpts.Only, []string{"README.md", "docs"})
	})

	t.Run("clone rejects conflicting shallow flags", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"clone", "openai/codex", "--shallow", "--no-shallow"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "--shallow")
		mustContain(t, stderr, "--no-shallow")
	})

	t.Run("clone rejects conflicting filter flags", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, calls, _ := runCLI(t, fake, []string{"clone", "openai/codex", "--filter", "blob:none", "--no-filter"})
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if calls != 0 {
			t.Fatalf("app factory calls = %d, want 0", calls)
		}
		mustContain(t, stderr, "--filter")
		mustContain(t, stderr, "--no-filter")
	})

	t.Run("link forwards flags", func(t *testing.T) {
		fake := &fakeApp{}
		code, _, stderr, _, _ := runCLI(t, fake, []string{
			"link",
			"software/codex",
			"--as", "codex-link",
			"--dir", "references",
			"--absolute",
			"--catalog", "references",
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if fake.linkOpts.Selector != "software/codex" {
			t.Fatalf("selector = %q, want %q", fake.linkOpts.Selector, "software/codex")
		}
		if fake.linkOpts.As != "codex-link" {
			t.Fatalf("as = %q, want %q", fake.linkOpts.As, "codex-link")
		}
		if fake.linkOpts.Dir != "references" {
			t.Fatalf("dir = %q, want %q", fake.linkOpts.Dir, "references")
		}
		if !fake.linkOpts.Absolute {
			t.Fatal("absolute = false, want true")
		}
		if fake.linkOpts.Catalog != "references" {
			t.Fatalf("catalog = %q, want %q", fake.linkOpts.Catalog, "references")
		}
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
