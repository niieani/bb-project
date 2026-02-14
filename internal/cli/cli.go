package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"bb-project/internal/app"
	"bb-project/internal/state"
	"github.com/spf13/cobra"
)

type appRunner interface {
	SetVerbose(verbose bool)
	RunInit(opts app.InitOptions) error
	RunScan(opts app.ScanOptions) (int, error)
	RunSync(opts app.SyncOptions) (int, error)
	RunFix(opts app.FixOptions) (int, error)
	RunStatus(jsonOut bool, include []string) (int, error)
	RunDoctor(include []string) (int, error)
	RunEnsure(include []string) (int, error)
	RunRepoPolicy(repoSelector string, autoPush bool) (int, error)
	RunRepoPreferredRemote(repoSelector string, preferredRemote string) (int, error)
	RunRepoPushAccessSet(repoSelector string, pushAccess string) (int, error)
	RunRepoPushAccessRefresh(repoSelector string) (int, error)
	RunCatalogAdd(name, root string) (int, error)
	RunCatalogRM(name string) (int, error)
	RunCatalogDefault(name string) (int, error)
	RunCatalogList() (int, error)
	RunConfig() error
}

type runDeps struct {
	userHomeDir func() (string, error)
	newApp      func(paths state.Paths, stdout io.Writer, stderr io.Writer) appRunner
}

type runtimeState struct {
	stdout io.Writer
	stderr io.Writer
	quiet  bool

	deps runDeps
	app  appRunner
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("exit code %d", e.code)
}

func (e *exitError) Unwrap() error {
	return e.err
}

func defaultRunDeps() runDeps {
	return runDeps{
		userHomeDir: os.UserHomeDir,
		newApp: func(paths state.Paths, stdout io.Writer, stderr io.Writer) appRunner {
			return app.New(paths, stdout, stderr)
		},
	}
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return runWithDeps(args, stdout, stderr, defaultRunDeps())
}

func NewRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
	runtime := &runtimeState{
		stdout: stdout,
		stderr: stderr,
		deps:   defaultRunDeps(),
	}
	cmd := newRootCommand(runtime)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd
}

func runWithDeps(args []string, stdout io.Writer, stderr io.Writer, deps runDeps) int {
	runtime := &runtimeState{
		stdout: stdout,
		stderr: stderr,
		deps:   deps,
	}

	cmd := newRootCommand(runtime)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	if err == nil {
		return 0
	}

	var codedErr *exitError
	if errors.As(err, &codedErr) {
		if codedErr.err != nil {
			fmt.Fprintln(stderr, codedErr.err)
		}
		if codedErr.code == 0 {
			return 2
		}
		return codedErr.code
	}

	fmt.Fprintln(stderr, err)
	return 2
}

func (r *runtimeState) appRunner() (appRunner, error) {
	if r.app != nil {
		return r.app, nil
	}

	home, err := r.deps.userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home: %w", err)
	}

	a := r.deps.newApp(state.NewPaths(home), r.stdout, r.stderr)
	a.SetVerbose(!r.quiet)
	r.app = a
	return r.app, nil
}

func newRootCommand(runtime *runtimeState) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bb",
		Short:         "Keep Git repositories consistent across machines.",
		Long:          "bb is a local-first CLI for repository bootstrap and safe cross-machine convergence.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return withExitCode(2, err)
			}
			return withExitCode(2, errors.New("a command is required"))
		},
	}

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return withExitCode(2, err)
	})

	cmd.PersistentFlags().BoolVarP(&runtime.quiet, "quiet", "q", false, "Suppress verbose bb logs.")

	cmd.AddCommand(
		newInitCommand(runtime),
		newScanCommand(runtime),
		newSyncCommand(runtime),
		newFixCommand(runtime),
		newStatusCommand(runtime),
		newDoctorCommand(runtime),
		newEnsureCommand(runtime),
		newRepoCommand(runtime),
		newCatalogCommand(runtime),
		newConfigCommand(runtime),
	)
	cmd.AddCommand(newCompletionCommand(runtime, cmd))

	return cmd
}

func withExitCode(code int, err error) error {
	if err == nil {
		if code == 0 {
			return nil
		}
		return &exitError{code: code}
	}
	if code == 0 {
		code = 2
	}
	return &exitError{code: code, err: err}
}

func newInitCommand(runtime *runtimeState) *cobra.Command {
	var catalog string
	var public bool
	var push bool
	var https bool

	cmd := &cobra.Command{
		Use:   "init [project]",
		Short: "Initialize or adopt a repository and register metadata.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			project := ""
			if len(args) == 1 {
				project = args[0]
			}
			err = runner.RunInit(app.InitOptions{
				Project: project,
				Catalog: catalog,
				Public:  public,
				Push:    push,
				HTTPS:   https,
			})
			return withExitCode(0, err)
		},
	}

	cmd.Flags().StringVar(&catalog, "catalog", "", "Select catalog instead of using the machine default.")
	cmd.Flags().BoolVar(&public, "public", false, "Create or register repository as public.")
	cmd.Flags().BoolVar(&push, "push", false, "Allow initial push/upstream setup when local commits exist.")
	cmd.Flags().BoolVar(&https, "https", false, "Use HTTPS remote protocol instead of SSH.")

	return cmd
}

func newScanCommand(runtime *runtimeState) *cobra.Command {
	var includeCatalogs []string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Discover repositories under catalogs and publish machine state.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunScan(app.ScanOptions{IncludeCatalogs: includeCatalogs})
			return withExitCode(code, err)
		},
	}

	cmd.Flags().StringArrayVar(&includeCatalogs, "include-catalog", nil, "Limit scope to selected catalogs (repeatable).")

	return cmd
}

func newSyncCommand(runtime *runtimeState) *cobra.Command {
	var includeCatalogs []string
	var push bool
	var notify bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Run observe, publish, and reconcile flow.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunSync(app.SyncOptions{
				IncludeCatalogs: includeCatalogs,
				Push:            push,
				Notify:          notify,
				DryRun:          dryRun,
			})
			return withExitCode(code, err)
		},
	}

	cmd.Flags().StringArrayVar(&includeCatalogs, "include-catalog", nil, "Limit scope to selected catalogs (repeatable).")
	cmd.Flags().BoolVar(&push, "push", false, "Allow pushing ahead commits when policy blocks by default.")
	cmd.Flags().BoolVar(&notify, "notify", false, "Emit notifications for unsyncable repositories.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show reconcile decisions without write-side sync actions.")

	return cmd
}

func newStatusCommand(runtime *runtimeState) *cobra.Command {
	var includeCatalogs []string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show last recorded machine repository state.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunStatus(jsonOut, includeCatalogs)
			return withExitCode(code, err)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print machine and repository state as JSON.")
	cmd.Flags().StringArrayVar(&includeCatalogs, "include-catalog", nil, "Limit scope to selected catalogs (repeatable).")

	return cmd
}

func newFixCommand(runtime *runtimeState) *cobra.Command {
	var includeCatalogs []string
	var message string
	var noRefresh bool

	cmd := &cobra.Command{
		Use:   "fix [project] [action]",
		Short: "Inspect repositories and apply context-aware fixes.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			opts := app.FixOptions{
				IncludeCatalogs: includeCatalogs,
				CommitMessage:   message,
				NoRefresh:       noRefresh,
			}
			if len(args) > 0 {
				opts.Project = args[0]
			}
			if len(args) > 1 {
				opts.Action = args[1]
			}
			code, err := runner.RunFix(opts)
			return withExitCode(code, err)
		},
	}

	cmd.Flags().StringArrayVar(&includeCatalogs, "include-catalog", nil, "Limit scope to selected catalogs (repeatable).")
	cmd.Flags().StringVar(&message, "message", "", "Commit message for stage-commit-push action (or 'auto').")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Use current machine snapshot without running a refresh scan first.")

	return cmd
}

func newDoctorCommand(runtime *runtimeState) *cobra.Command {
	var includeCatalogs []string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report unsyncable repositories and reasons.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunDoctor(includeCatalogs)
			return withExitCode(code, err)
		},
	}

	cmd.Flags().StringArrayVar(&includeCatalogs, "include-catalog", nil, "Limit scope to selected catalogs (repeatable).")

	return cmd
}

func newEnsureCommand(runtime *runtimeState) *cobra.Command {
	var includeCatalogs []string

	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Alias for sync convergence over selected catalogs.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunEnsure(includeCatalogs)
			return withExitCode(code, err)
		},
	}

	cmd.Flags().StringArrayVar(&includeCatalogs, "include-catalog", nil, "Limit scope to selected catalogs (repeatable).")

	return cmd
}

func newRepoCommand(runtime *runtimeState) *cobra.Command {
	repoCmd := &cobra.Command{
		Use:           "repo",
		Short:         "Manage repository metadata and policy settings.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return withExitCode(2, err)
			}
			return withExitCode(2, errors.New("repo subcommand is required"))
		},
	}

	var autoPushRaw string
	policyCmd := &cobra.Command{
		Use:   "policy <repo>",
		Short: "Set repository auto-push policy.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}

			autoPush, err := strconv.ParseBool(autoPushRaw)
			if err != nil {
				return withExitCode(2, fmt.Errorf("invalid --auto-push value %q", autoPushRaw))
			}

			code, err := runner.RunRepoPolicy(args[0], autoPush)
			return withExitCode(code, err)
		},
	}

	policyCmd.Flags().StringVar(&autoPushRaw, "auto-push", "", "Set auto-push policy (true|false).")
	_ = policyCmd.MarkFlagRequired("auto-push")

	var preferredRemote string
	remoteCmd := &cobra.Command{
		Use:   "remote <repo>",
		Short: "Set repository preferred remote for sync/fix operations.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunRepoPreferredRemote(args[0], preferredRemote)
			return withExitCode(code, err)
		},
	}
	remoteCmd.Flags().StringVar(&preferredRemote, "preferred-remote", "", "Preferred remote name for this repository (for example origin or upstream).")
	_ = remoteCmd.MarkFlagRequired("preferred-remote")

	var pushAccess string
	accessSetCmd := &cobra.Command{
		Use:   "access-set <repo>",
		Short: "Set cached repository push access (read_write|read_only|unknown).",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunRepoPushAccessSet(args[0], pushAccess)
			return withExitCode(code, err)
		},
	}
	accessSetCmd.Flags().StringVar(&pushAccess, "push-access", "", "Cached push access level (read_write|read_only|unknown).")
	_ = accessSetCmd.MarkFlagRequired("push-access")

	accessRefreshCmd := &cobra.Command{
		Use:   "access-refresh <repo>",
		Short: "Probe and refresh cached repository push access.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunRepoPushAccessRefresh(args[0])
			return withExitCode(code, err)
		},
	}

	repoCmd.AddCommand(policyCmd, remoteCmd, accessSetCmd, accessRefreshCmd)
	return repoCmd
}

func newCatalogCommand(runtime *runtimeState) *cobra.Command {
	catalogCmd := &cobra.Command{
		Use:           "catalog",
		Short:         "Manage machine catalogs and default catalog selection.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return withExitCode(2, err)
			}
			return withExitCode(2, errors.New("catalog subcommand is required"))
		},
	}

	addCmd := &cobra.Command{
		Use:   "add <name> <root>",
		Short: "Add catalog root to current machine.",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunCatalogAdd(args[0], args[1])
			return withExitCode(code, err)
		},
	}

	rmCmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove catalog from current machine.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunCatalogRM(args[0])
			return withExitCode(code, err)
		},
	}

	defaultCmd := &cobra.Command{
		Use:   "default <name>",
		Short: "Set machine default catalog.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunCatalogDefault(args[0])
			return withExitCode(code, err)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured catalogs and mark default.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			code, err := runner.RunCatalogList()
			return withExitCode(code, err)
		},
	}

	catalogCmd.AddCommand(addCmd, rmCmd, defaultCmd, listCmd)
	return catalogCmd
}

func newConfigCommand(runtime *runtimeState) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Launch interactive configuration wizard.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runner, err := runtime.appRunner()
			if err != nil {
				return withExitCode(2, err)
			}
			return withExitCode(0, runner.RunConfig())
		},
	}
}

func newCompletionCommand(runtime *runtimeState, root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts.",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(_ *cobra.Command, args []string) error {
			var err error
			switch args[0] {
			case "bash":
				err = root.GenBashCompletionV2(runtime.stdout, true)
			case "zsh":
				err = root.GenZshCompletion(runtime.stdout)
			case "fish":
				err = root.GenFishCompletion(runtime.stdout, true)
			case "powershell":
				err = root.GenPowerShellCompletionWithDesc(runtime.stdout)
			default:
				err = fmt.Errorf("unsupported shell %q", args[0])
			}
			return withExitCode(0, err)
		},
	}
}
