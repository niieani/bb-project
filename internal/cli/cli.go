package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"bb-project/internal/app"
	"bb-project/internal/state"
)

type helpItem struct {
	Name        string
	Summary     string
	Usage       []string
	Details     []string
	Flags       []helpField
	Subcommands []helpField
	Examples    []string
}

type helpField struct {
	Name        string
	Description string
}

var topLevelHelpOrder = []string{
	"init",
	"scan",
	"sync",
	"status",
	"doctor",
	"ensure",
	"repo",
	"catalog",
	"config",
	"help",
}

var helpTopics = map[string]helpItem{
	"init": {
		Name:    "init",
		Summary: "Initialize or adopt a repository in a catalog and register metadata.",
		Usage: []string{
			"bb init [project] [--catalog <name>] [--public] [--push] [--https]",
		},
		Details: []string{
			"If project is omitted, bb infers it from the current directory when possible.",
		},
		Flags: []helpField{
			{Name: "--catalog <name>", Description: "Select catalog instead of using the machine default."},
			{Name: "--public", Description: "Create or register repository as public (default is private)."},
			{Name: "--push", Description: "Allow initial push/upstream setup when local commits exist."},
			{Name: "--https", Description: "Use HTTPS remote protocol instead of SSH."},
		},
		Examples: []string{
			"bb init api",
			"bb init api --catalog software --public",
		},
	},
	"scan": {
		Name:    "scan",
		Summary: "Discover repositories under catalogs and publish observed machine state.",
		Usage: []string{
			"bb scan [--include-catalog <name> ...]",
		},
		Flags: []helpField{
			{Name: "--include-catalog <name>", Description: "Limit scan scope to selected catalogs (repeatable)."},
		},
		Examples: []string{
			"bb scan",
			"bb scan --include-catalog software",
		},
	},
	"sync": {
		Name:    "sync",
		Summary: "Run observe, publish, and reconcile flow to converge repositories safely.",
		Usage: []string{
			"bb sync [--include-catalog <name> ...] [--push] [--notify] [--dry-run]",
		},
		Flags: []helpField{
			{Name: "--include-catalog <name>", Description: "Limit sync scope to selected catalogs (repeatable)."},
			{Name: "--push", Description: "Allow pushing ahead commits when repo policy blocks by default."},
			{Name: "--notify", Description: "Emit notifications for unsyncable repositories."},
			{Name: "--dry-run", Description: "Show reconcile decisions without write-side sync actions."},
		},
		Examples: []string{
			"bb sync --notify",
			"bb sync --include-catalog software --dry-run",
		},
	},
	"status": {
		Name:    "status",
		Summary: "Show last recorded machine repository state.",
		Usage: []string{
			"bb status [--json] [--include-catalog <name> ...]",
		},
		Flags: []helpField{
			{Name: "--json", Description: "Print machine and repository state as JSON."},
			{Name: "--include-catalog <name>", Description: "Limit output to selected catalogs (repeatable)."},
		},
		Examples: []string{
			"bb status",
			"bb status --json --include-catalog software",
		},
	},
	"doctor": {
		Name:    "doctor",
		Summary: "Report unsyncable repositories and reasons.",
		Usage: []string{
			"bb doctor [--include-catalog <name> ...]",
		},
		Flags: []helpField{
			{Name: "--include-catalog <name>", Description: "Limit checks to selected catalogs (repeatable)."},
		},
		Examples: []string{
			"bb doctor",
			"bb doctor --include-catalog software",
		},
	},
	"ensure": {
		Name:    "ensure",
		Summary: "Alias for sync convergence over selected catalogs.",
		Usage: []string{
			"bb ensure [--include-catalog <name> ...]",
		},
		Flags: []helpField{
			{Name: "--include-catalog <name>", Description: "Limit convergence to selected catalogs (repeatable)."},
		},
		Examples: []string{
			"bb ensure",
			"bb ensure --include-catalog software",
		},
	},
	"repo": {
		Name:    "repo",
		Summary: "Manage repository metadata and policy settings.",
		Usage: []string{
			"bb repo policy <repo> --auto-push=<true|false>",
		},
		Subcommands: []helpField{
			{Name: "policy <repo> --auto-push=<true|false>", Description: "Set auto_push policy by repo_id or unique repo name."},
		},
		Examples: []string{
			"bb repo policy github.com/you/service --auto-push=false",
			"bb repo policy service --auto-push=true",
		},
	},
	"catalog": {
		Name:    "catalog",
		Summary: "Manage machine catalogs and default catalog selection.",
		Usage: []string{
			"bb catalog add <name> <root>",
			"bb catalog rm <name>",
			"bb catalog default <name>",
			"bb catalog list",
		},
		Subcommands: []helpField{
			{Name: "add <name> <root>", Description: "Add catalog root to current machine."},
			{Name: "rm <name>", Description: "Remove catalog from current machine."},
			{Name: "default <name>", Description: "Set machine default catalog."},
			{Name: "list", Description: "List configured catalogs and mark default."},
		},
		Examples: []string{
			"bb catalog add software /Volumes/Projects/Software",
			"bb catalog default software",
		},
	},
	"config": {
		Name:    "config",
		Summary: "Launch interactive configuration wizard.",
		Usage: []string{
			"bb config",
		},
		Details: []string{
			"Requires an interactive terminal.",
		},
	},
	"help": {
		Name:    "help",
		Summary: "Show general help or command-specific help.",
		Usage: []string{
			"bb help",
			"bb help <command>",
		},
		Examples: []string{
			"bb help",
			"bb help sync",
		},
	},
	"repo policy": {
		Name:    "repo policy",
		Summary: "Set repository auto_push policy.",
		Usage: []string{
			"bb repo policy <repo> --auto-push=<true|false>",
		},
		Examples: []string{
			"bb repo policy service --auto-push=true",
		},
	},
	"catalog add": {
		Name:    "catalog add",
		Summary: "Add a machine catalog root.",
		Usage: []string{
			"bb catalog add <name> <root>",
		},
	},
	"catalog rm": {
		Name:    "catalog rm",
		Summary: "Remove a machine catalog.",
		Usage: []string{
			"bb catalog rm <name>",
		},
	},
	"catalog default": {
		Name:    "catalog default",
		Summary: "Set machine default catalog.",
		Usage: []string{
			"bb catalog default <name>",
		},
	},
	"catalog list": {
		Name:    "catalog list",
		Summary: "List machine catalogs.",
		Usage: []string{
			"bb catalog list",
		},
	},
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	parsedArgs, quiet, helpRequested := stripGlobalFlags(args)
	args = parsedArgs

	if helpRequested {
		if len(args) > 0 && args[0] == "help" {
			return runHelp(args[1:], stdout, stderr)
		}
		if len(args) > 0 {
			return runHelp(args[:1], stdout, stderr)
		}
		return runHelp(nil, stdout, stderr)
	}

	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	cmd := args[0]
	rest := args[1:]
	if cmd == "help" {
		return runHelp(rest, stdout, stderr)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "resolve home: %v\n", err)
		return 2
	}
	a := app.New(state.NewPaths(home), stdout, stderr)
	a.SetVerbose(!quiet)
	var code int
	switch cmd {
	case "init":
		opts, err := parseInitArgs(rest)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		err = a.RunInit(opts)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	case "scan":
		include, err := parseIncludeCatalogs(rest)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		code, err = a.RunScan(app.ScanOptions{IncludeCatalogs: include})
	case "sync":
		opts, err := parseSyncArgs(rest)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		code, err = a.RunSync(opts)
	case "status":
		jsonOut := false
		include := []string{}
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--json":
				jsonOut = true
			case "--include-catalog":
				i++
				if i >= len(rest) {
					fmt.Fprintln(stderr, "--include-catalog requires a value")
					return 2
				}
				include = append(include, rest[i])
			default:
				fmt.Fprintf(stderr, "unknown status arg %q\n", rest[i])
				return 2
			}
		}
		code, err = a.RunStatus(jsonOut, include)
	case "doctor":
		include, err := parseIncludeCatalogs(rest)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		code, err = a.RunDoctor(include)
	case "ensure":
		include, err := parseIncludeCatalogs(rest)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		code, err = a.RunEnsure(include)
	case "repo":
		code, err = runRepoSubcommand(a, rest)
	case "catalog":
		code, err = runCatalogSubcommand(a, rest)
	case "config":
		if err := parseConfigArgs(rest); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if err := a.RunConfig(); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", cmd)
		printUsage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		if code == 0 {
			return 2
		}
		return code
	}
	return code
}

func runHelp(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	topic := strings.Join(args, " ")
	if printTopicHelp(stdout, topic) {
		return 0
	}
	fmt.Fprintf(stderr, "unknown help topic %q\n", topic)
	printUsage(stderr)
	return 2
}

func runRepoSubcommand(a *app.App, args []string) (int, error) {
	if len(args) < 1 {
		return 2, errors.New("repo subcommand required")
	}
	if args[0] != "policy" {
		return 2, fmt.Errorf("unknown repo command %q", args[0])
	}
	if len(args) < 3 {
		return 2, errors.New("usage: bb repo policy <repo> --auto-push=<true|false>")
	}
	repo := args[1]
	autoPush := false
	found := false
	for _, arg := range args[2:] {
		if strings.HasPrefix(arg, "--auto-push=") {
			v := strings.TrimPrefix(arg, "--auto-push=")
			b, err := strconv.ParseBool(v)
			if err != nil {
				return 2, fmt.Errorf("invalid --auto-push value %q", v)
			}
			autoPush = b
			found = true
			continue
		}
		return 2, fmt.Errorf("unknown argument %q", arg)
	}
	if !found {
		return 2, errors.New("--auto-push is required")
	}
	return a.RunRepoPolicy(repo, autoPush)
}

func runCatalogSubcommand(a *app.App, args []string) (int, error) {
	if len(args) < 1 {
		return 2, errors.New("catalog subcommand required")
	}
	switch args[0] {
	case "add":
		if len(args) != 3 {
			return 2, errors.New("usage: bb catalog add <name> <root>")
		}
		return a.RunCatalogAdd(args[1], args[2])
	case "rm":
		if len(args) != 2 {
			return 2, errors.New("usage: bb catalog rm <name>")
		}
		return a.RunCatalogRM(args[1])
	case "default":
		if len(args) != 2 {
			return 2, errors.New("usage: bb catalog default <name>")
		}
		return a.RunCatalogDefault(args[1])
	case "list":
		if len(args) != 1 {
			return 2, errors.New("usage: bb catalog list")
		}
		return a.RunCatalogList()
	default:
		return 2, fmt.Errorf("unknown catalog command %q", args[0])
	}
}

func parseInitArgs(args []string) (app.InitOptions, error) {
	var out app.InitOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--public":
			out.Public = true
		case arg == "--push":
			out.Push = true
		case arg == "--https":
			out.HTTPS = true
		case arg == "--catalog":
			i++
			if i >= len(args) {
				return app.InitOptions{}, errors.New("--catalog requires value")
			}
			out.Catalog = args[i]
		case strings.HasPrefix(arg, "--catalog="):
			out.Catalog = strings.TrimPrefix(arg, "--catalog=")
		case strings.HasPrefix(arg, "-"):
			return app.InitOptions{}, fmt.Errorf("unknown init flag %q", arg)
		default:
			if out.Project != "" {
				return app.InitOptions{}, errors.New("init accepts at most one project argument")
			}
			out.Project = arg
		}
	}
	return out, nil
}

func parseIncludeCatalogs(args []string) ([]string, error) {
	out := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--include-catalog":
			i++
			if i >= len(args) {
				return nil, errors.New("--include-catalog requires value")
			}
			out = append(out, args[i])
		case strings.HasPrefix(arg, "--include-catalog="):
			out = append(out, strings.TrimPrefix(arg, "--include-catalog="))
		case arg == "":
			continue
		default:
			return nil, fmt.Errorf("unknown argument %q", arg)
		}
	}
	return out, nil
}

func parseSyncArgs(args []string) (app.SyncOptions, error) {
	var out app.SyncOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--push":
			out.Push = true
		case arg == "--notify":
			out.Notify = true
		case arg == "--dry-run":
			out.DryRun = true
		case arg == "--include-catalog":
			i++
			if i >= len(args) {
				return app.SyncOptions{}, errors.New("--include-catalog requires value")
			}
			out.IncludeCatalogs = append(out.IncludeCatalogs, args[i])
		case strings.HasPrefix(arg, "--include-catalog="):
			out.IncludeCatalogs = append(out.IncludeCatalogs, strings.TrimPrefix(arg, "--include-catalog="))
		default:
			return app.SyncOptions{}, fmt.Errorf("unknown sync arg %q", arg)
		}
	}
	return out, nil
}

func parseConfigArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unknown config arg %q", args[0])
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "bb keeps Git repositories consistent across machines.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bb [global flags] <command> [args]")
	fmt.Fprintln(w, "  bb help [command]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global flags:")
	fmt.Fprintln(w, "  -q, --quiet  Suppress verbose bb logs.")
	fmt.Fprintln(w, "  -h, --help   Show help (general or command-specific).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	rows := make([]helpField, 0, len(topLevelHelpOrder))
	for _, name := range topLevelHelpOrder {
		topic, ok := helpTopics[name]
		if !ok {
			continue
		}
		rows = append(rows, helpField{Name: name, Description: topic.Summary})
	}
	printAlignedHelpFields(w, rows)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bb init api --catalog software")
	fmt.Fprintln(w, "  bb sync --notify --include-catalog software")
	fmt.Fprintln(w, "  bb status --json")
	fmt.Fprintln(w, "  bb help sync")
}

func printTopicHelp(w io.Writer, topic string) bool {
	doc, ok := helpTopics[topic]
	if !ok {
		return false
	}
	fmt.Fprintf(w, "Command: %s\n", doc.Name)
	fmt.Fprintln(w)
	if len(doc.Usage) == 1 {
		fmt.Fprintf(w, "Usage: %s\n", doc.Usage[0])
	} else {
		fmt.Fprintln(w, "Usage:")
		for _, usage := range doc.Usage {
			fmt.Fprintf(w, "  %s\n", usage)
		}
	}
	if doc.Summary != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Summary:")
		fmt.Fprintf(w, "  %s\n", doc.Summary)
	}
	if len(doc.Details) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Details:")
		for _, detail := range doc.Details {
			fmt.Fprintf(w, "  %s\n", detail)
		}
	}
	if len(doc.Flags) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Flags:")
		printAlignedHelpFields(w, doc.Flags)
	}
	if len(doc.Subcommands) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Subcommands:")
		printAlignedHelpFields(w, doc.Subcommands)
	}
	if len(doc.Examples) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		for _, example := range doc.Examples {
			fmt.Fprintf(w, "  %s\n", example)
		}
	}
	return true
}

func printAlignedHelpFields(w io.Writer, fields []helpField) {
	width := 0
	for _, field := range fields {
		if len(field.Name) > width {
			width = len(field.Name)
		}
	}
	for _, field := range fields {
		fmt.Fprintf(w, "  %-*s  %s\n", width, field.Name, field.Description)
	}
}

func stripGlobalFlags(args []string) ([]string, bool, bool) {
	out := make([]string, 0, len(args))
	quiet := false
	help := false
	for _, arg := range args {
		switch arg {
		case "--quiet", "-q":
			quiet = true
		case "--help", "-h":
			help = true
		default:
			out = append(out, arg)
		}
	}
	return out, quiet, help
}
