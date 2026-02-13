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

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	parsedArgs, quiet := stripGlobalFlags(args)
	args = parsedArgs

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "resolve home: %v\n", err)
		return 2
	}
	a := app.New(state.NewPaths(home), stdout, stderr)
	a.SetVerbose(!quiet)

	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	cmd := args[0]
	rest := args[1:]
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
	fmt.Fprintln(w, "usage: bb <command> [args]")
	fmt.Fprintln(w, "global flags: --quiet")
	fmt.Fprintln(w, "commands: init sync status doctor scan ensure repo catalog config")
}

func stripGlobalFlags(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	quiet := false
	for _, arg := range args {
		switch arg {
		case "--quiet", "-q":
			quiet = true
		default:
			out = append(out, arg)
		}
	}
	return out, quiet
}
