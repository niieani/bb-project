package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bb-project/internal/domain"
	"bb-project/internal/state"
)

func TestRunFixTargetedSkipsGlobalRiskAndPushAccessCollection(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 16, 9, 0, 0, 0, time.UTC)
	home := t.TempDir()
	paths := state.NewPaths(home)

	cfg := state.DefaultConfig()
	cfg.GitHub.Owner = "you"
	if err := state.SaveConfig(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	catalogRoot := filepath.Join(home, "software")
	apiPath := filepath.Join(catalogRoot, "api")
	webPath := filepath.Join(catalogRoot, "web")
	if err := state.EnsureDir(apiPath); err != nil {
		t.Fatalf("mkdir api: %v", err)
	}
	if err := state.EnsureDir(webPath); err != nil {
		t.Fatalf("mkdir web: %v", err)
	}

	apiRecord := domain.MachineRepoRecord{
		RepoKey:   "software/api",
		Name:      "api",
		Catalog:   "software",
		Path:      apiPath,
		OriginURL: "https://github.com/you/api.git",
		Syncable:  true,
	}
	apiRecord.StateHash = domain.ComputeStateHash(apiRecord)
	webRecord := domain.MachineRepoRecord{
		RepoKey:   "software/web",
		Name:      "web",
		Catalog:   "software",
		Path:      webPath,
		OriginURL: "https://github.com/you/web.git",
		Syncable:  true,
	}
	webRecord.StateHash = domain.ComputeStateHash(webRecord)

	machine := state.BootstrapMachine("fix-targeted-host", "fix-targeted-host", now)
	machine.DefaultCatalog = "software"
	machine.Catalogs = []domain.Catalog{{Name: "software", Root: catalogRoot, RepoPathDepth: 1}}
	machine.Repos = []domain.MachineRepoRecord{apiRecord, webRecord}
	machine.LastScanAt = now
	machine.LastScanCatalogs = []string{"software"}
	machine.UpdatedAt = now
	if err := state.SaveMachine(paths, machine); err != nil {
		t.Fatalf("save machine: %v", err)
	}

	for _, meta := range []domain.RepoMetadataFile{
		{
			RepoKey:          "software/api",
			Name:             "api",
			OriginURL:        "https://github.com/you/api.git",
			Visibility:       domain.VisibilityPrivate,
			PreferredCatalog: "software",
			AutoPush:         domain.AutoPushModeDisabled,
			PushAccess:       domain.PushAccessUnknown,
		},
		{
			RepoKey:          "software/web",
			Name:             "web",
			OriginURL:        "https://github.com/you/web.git",
			Visibility:       domain.VisibilityPrivate,
			PreferredCatalog: "software",
			AutoPush:         domain.AutoPushModeDisabled,
			PushAccess:       domain.PushAccessUnknown,
		},
	} {
		if err := state.SaveRepoMetadata(paths, meta); err != nil {
			t.Fatalf("save metadata for %s: %v", meta.RepoKey, err)
		}
	}

	var stdout bytes.Buffer
	app := New(paths, &stdout, &bytes.Buffer{})
	app.Hostname = func() (string, error) { return "fix-targeted-host", nil }
	app.Now = func() time.Time { return now }
	app.SetVerbose(false)

	logs := make([]string, 0, 16)
	restore := app.setLogObserver(func(line string) {
		logs = append(logs, line)
	})
	defer restore()

	code, err := app.runFix(FixOptions{Project: apiPath})
	if err != nil {
		t.Fatalf("runFix returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("runFix code = %d, want 0", code)
	}

	joinedLogs := strings.Join(logs, "\n")
	if strings.Contains(joinedLogs, "collecting risk checks for 2 repositories") {
		t.Fatalf("targeted fix should not collect risk for all repos, logs: %s", joinedLogs)
	}
	if strings.Contains(joinedLogs, "verifying push access for 2 repositories with unknown access") {
		t.Fatalf("targeted fix should not verify unknown push access for all repos, logs: %s", joinedLogs)
	}
}
