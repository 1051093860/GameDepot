package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/1051093860/gamedepot/internal/commands"
	"github.com/1051093860/gamedepot/internal/smoke"
	"github.com/1051093860/gamedepot/internal/ueapi"
	"github.com/1051093860/gamedepot/internal/ueplugin"
)

var version = "v0.13.0-pointer-refs-clone"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "init":
		return runInit(ctx, os.Args[2:])
	case "clone":
		return runClone(ctx, os.Args[2:])
	case "doctor":
		return commands.Doctor(ctx, ".")
	case "update":
		fs := flag.NewFlagSet("update", flag.ExitOnError)
		force := fs.Bool("force", false, "discard local conflicting Content changes")
		strict := fs.Bool("strict", false, "abort if any local Content changes exist")
		_ = fs.Parse(os.Args[2:])
		return commands.Update(ctx, ".", commands.UpdateOptions{Force: *force, Strict: *strict})
	case "conflicts":
		fs := flag.NewFlagSet("conflicts", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(os.Args[2:])
		return commands.Conflicts(ctx, ".", *jsonOut)
	case "resolve":
		fs := flag.NewFlagSet("resolve", flag.ExitOnError)
		useRemote := fs.Bool("remote", false, "use remote version")
		useLocal := fs.Bool("local", false, "keep local version and publish it")
		abort := fs.Bool("abort", false, "clear active conflict state")
		_ = fs.Parse(os.Args[2:])
		if *abort {
			return commands.ResolveConflict(ctx, ".", "", "abort")
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot resolve <Content/path> (--remote|--local)")
		}
		if *useRemote == *useLocal {
			return fmt.Errorf("choose exactly one of --remote or --local")
		}
		decision := "remote"
		if *useLocal {
			decision = "local"
		}
		return commands.ResolveConflict(ctx, ".", fs.Arg(0), decision)
	case "publish":
		fs := flag.NewFlagSet("publish", flag.ExitOnError)
		msg := fs.String("m", "", "commit message")
		dryRun := fs.Bool("dry-run", false, "print publish plan without changing files")
		_ = fs.Parse(os.Args[2:])
		return commands.Publish(ctx, ".", *msg, commands.PublishOptions{DryRun: *dryRun})
	case "history":
		fs := flag.NewFlagSet("history", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot history <Content/path> [--json]")
		}
		return commands.HistoryWithOptions(ctx, ".", fs.Arg(0), commands.HistoryOptions{JSON: *jsonOut})
	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(os.Args[2:])
		return commands.Status(ctx, ".", *jsonOut)
	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		localOnly := fs.Bool("local-only", false, "skip remote blob checks")
		remoteOnly := fs.Bool("remote-only", false, "skip local workspace checks")
		_ = fs.Parse(os.Args[2:])
		return commands.VerifyWithOptions(ctx, ".", commands.VerifyOptions{LocalOnly: *localOnly, RemoteOnly: *remoteOnly})
	case "smoke-test", "smoke":
		fs := flag.NewFlagSet("smoke-test", flag.ExitOnError)
		var opts smoke.PointerRefsOptions
		smoke.RegisterPointerRefsFlags(fs, &opts)
		_ = fs.Parse(os.Args[2:])
		return smoke.RunPointerRefs(ctx, opts)
	case "checkout":
		fs := flag.NewFlagSet("checkout", flag.ExitOnError)
		force := fs.Bool("force", false, "discard local unsubmitted blob changes while switching versions")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot checkout <git-ref> [--force]")
		}
		return commands.Checkout(ctx, ".", fs.Arg(0), commands.CheckoutOptions{Force: *force})
	case "config":
		return runConfig(ctx, os.Args[2:])
	case "asset":
		return runAsset(ctx, os.Args[2:])
	case "store":
		return runStore(ctx, os.Args[2:])
	case "gc":
		return runGC(ctx, os.Args[2:])
	case "daemon", "serve":
		return runDaemon(ctx, os.Args[1], os.Args[2:])
	case "ue-plugin":
		return runUEPlugin(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return nil

	// Backward-compatible aliases hidden from help.
	case "asset-status":
		return runAsset(ctx, append([]string{"status"}, os.Args[2:]...))
	case "restore-version":
		return runAsset(ctx, append([]string{"restore"}, os.Args[2:]...))
	case "revert-assets":
		return runAsset(ctx, append([]string{"revert"}, os.Args[2:]...))
	case "restore":
		return runBlobRestore(ctx, os.Args[2:])
	case "pull", "sync", "submit", "push":
		return fmt.Errorf("%q was removed in the pointer-refs CLI; use `gamedepot update` or `gamedepot publish -m <message>`", os.Args[1])
	case "rules":
		return fmt.Errorf("%q was removed; Content/** is always managed by GameDepot pointer refs", os.Args[1])
	case "classify", "list", "ls", "repair-current-blob", "git-config", "project", "cmd-smoke-test", "ue-api-smoke-test", "v08-core-smoke-test", "v08-api-smoke-test", "ue-plugin-smoke-test", "remote-smoke-test", "team-smoke-test":
		return fmt.Errorf("%q has been removed or hidden; use the structured commands shown by `gamedepot help`", os.Args[1])
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func runInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	projectID := fs.String("project", "", "project id; defaults to .uproject name")
	remoteURL := fs.String("remote", "", "Git remote URL to configure, for example a new empty GitHub repo")
	remoteName := fs.String("remote-name", "origin", "Git remote name")
	branch := fs.String("branch", "", "local branch/upstream name to configure, for example main")
	noPlugin := fs.Bool("no-plugin", false, "do not install the UE plugin")
	overwritePlugin := fs.Bool("overwrite-plugin", true, "overwrite existing GameDepotUE plugin")
	_ = fs.Parse(reorderFlags(args, map[string]bool{"no-plugin": true, "overwrite-plugin": true}))
	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}
	if err := commands.ProjectInitUE(ctx, root, commands.ProjectInitUEOptions{Project: *projectID, Profile: "local", RemoteURL: *remoteURL, RemoteName: *remoteName, Branch: *branch}); err != nil {
		return err
	}
	if !*noPlugin {
		if err := ueplugin.Install(ueplugin.InstallOptions{Project: root, Overwrite: *overwritePlugin, EnablePlugin: true, WriteProjectConfig: true, VerifyAfter: true}); err != nil {
			return err
		}
	}
	return nil
}

func runClone(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("clone", flag.ExitOnError)
	branch := fs.String("branch", "", "branch to clone or initialize, for example main")
	remoteName := fs.String("remote-name", "origin", "Git remote name")
	projectID := fs.String("project", "", "project id used if cloning a plain UE repo without GameDepot config")
	noUpdate := fs.Bool("no-update", false, "skip post-clone gamedepot update")
	_ = fs.Parse(reorderFlags(args, map[string]bool{"no-update": true}))
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: gamedepot clone [--branch <branch>] [--remote-name origin] [--no-update] <remote-url> [dir]")
	}
	remoteURL := fs.Arg(0)
	dest := ""
	if fs.NArg() > 1 {
		dest = fs.Arg(1)
	}
	_, err := commands.Clone(ctx, remoteURL, dest, commands.CloneOptions{Branch: *branch, RemoteName: *remoteName, Project: *projectID, NoUpdate: *noUpdate})
	return err
}

func reorderFlags(args []string, boolFlags map[string]bool) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if eq := strings.Index(name, "="); eq >= 0 {
				continue
			}
			if !boolFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positionals = append(positionals, a)
	}
	return append(flags, positionals...)
}

func runConfig(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gamedepot config <path|user|profiles|add-local|add-oss|add-s3|set-credentials|use|project-use>")
	}
	switch args[0] {
	case "path":
		return commands.ConfigPath(ctx, ".")
	case "user":
		fs := flag.NewFlagSet("config user", flag.ExitOnError)
		name := fs.String("name", "", "global display name")
		email := fs.String("email", "", "global email")
		_ = fs.Parse(args[1:])
		return commands.ConfigUser(ctx, *name, *email, "")
	case "profiles":
		return commands.ConfigProfiles(ctx)
	case "add-local":
		fs := flag.NewFlagSet("config add-local", flag.ExitOnError)
		path := fs.String("path", ".gamedepot/remote_blobs", "local blob path")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot config add-local <name> [--path <path>]")
		}
		return commands.ConfigAddLocal(ctx, fs.Arg(0), *path)
	case "add-oss":
		fs := flag.NewFlagSet("config add-oss", flag.ExitOnError)
		region := fs.String("region", "cn-hangzhou", "OSS region")
		bucket := fs.String("bucket", "", "OSS bucket")
		endpoint := fs.String("endpoint", "", "OSS endpoint")
		internal := fs.Bool("internal", false, "use internal endpoint")
		accel := fs.Bool("accelerate", false, "use accelerate endpoint")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot config add-oss <name> --bucket <bucket> [--region <region>]")
		}
		return commands.ConfigAddOSS(ctx, fs.Arg(0), *region, *bucket, *endpoint, *internal, *accel)
	case "add-s3":
		fs := flag.NewFlagSet("config add-s3", flag.ExitOnError)
		endpoint := fs.String("endpoint", "", "S3 endpoint")
		region := fs.String("region", "us-east-1", "S3 region")
		bucket := fs.String("bucket", "", "S3 bucket")
		fps := fs.Bool("force-path-style", false, "force path-style")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot config add-s3 <name> --endpoint <url> --bucket <bucket>")
		}
		return commands.ConfigAddS3(ctx, fs.Arg(0), *endpoint, *region, *bucket, *fps)
	case "set-credentials":
		fs := flag.NewFlagSet("config set-credentials", flag.ExitOnError)
		id := fs.String("access-key-id", "", "")
		sec := fs.String("access-key-secret", "", "")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot config set-credentials <profile>")
		}
		return commands.ConfigSetCredentials(ctx, fs.Arg(0), *id, *sec)
	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: gamedepot config use <profile>")
		}
		return commands.ConfigUse(ctx, args[1])
	case "project-use":
		if len(args) < 2 {
			return fmt.Errorf("usage: gamedepot config project-use <profile>")
		}
		return commands.ConfigProjectUse(ctx, ".", args[1])
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func runAsset(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gamedepot asset <status|history|restore|revert>")
	}
	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("asset status", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "")
		rec := fs.Bool("recursive", false, "")
		_ = fs.Parse(args[1:])
		target := ""
		if fs.NArg() > 0 {
			target = fs.Arg(0)
		}
		return commands.AssetStatusCommand(ctx, ".", target, commands.AssetStatusOptions{JSON: *jsonOut, Recursive: *rec})
	case "history":
		fs := flag.NewFlagSet("asset history", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "print JSON")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot asset history <path> [--json]")
		}
		return commands.HistoryWithOptions(ctx, ".", fs.Arg(0), commands.HistoryOptions{JSON: *jsonOut})
	case "restore":
		fs := flag.NewFlagSet("asset restore", flag.ExitOnError)
		commit := fs.String("commit", "", "source commit")
		force := fs.Bool("force", false, "")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot asset restore <path> --commit <commit>")
		}
		return commands.RestoreVersion(ctx, ".", fs.Arg(0), *commit, *force)
	case "revert":
		fs := flag.NewFlagSet("asset revert", flag.ExitOnError)
		force := fs.Bool("force", false, "")
		_ = fs.Parse(args[1:])
		return commands.RevertAssets(ctx, ".", fs.Args(), *force)
	default:
		return fmt.Errorf("unknown asset subcommand %q", args[0])
	}
}

func runStore(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gamedepot store <info|check>")
	}
	if args[0] == "info" {
		return commands.StoreInfo(ctx, ".")
	}
	if args[0] == "check" {
		return commands.StoreCheck(ctx, ".")
	}
	return fmt.Errorf("unknown store subcommand %q", args[0])
}

func runGC(ctx context.Context, args []string) error {
	opts := commands.GCOptions{DryRun: true}
	if len(args) > 0 && (args[0] == "run" || args[0] == "execute") {
		opts.DryRun = false
		args = args[1:]
	} else if len(args) > 0 && args[0] == "preview" {
		opts.DryRun = true
		args = args[1:]
	}
	fs := flag.NewFlagSet("gc", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "")
	dry := fs.Bool("dry-run", opts.DryRun, "")
	exec := fs.Bool("execute", !opts.DryRun, "")
	_ = fs.Parse(args)
	opts.JSON = *jsonOut
	opts.DryRun = *dry
	if *exec {
		opts.DryRun = false
	}
	return commands.GC(ctx, ".", opts)
}

func runBlobRestore(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	sha := fs.String("sha256", "", "")
	force := fs.Bool("force", false, "")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: gamedepot asset restore <path> --commit <commit>")
	}
	return commands.Restore(ctx, ".", fs.Arg(0), *sha, *force)
}

func runDaemon(ctx context.Context, name string, args []string) error {
	if name == "daemon" && len(args) > 0 && args[0] == "stop" {
		fs := flag.NewFlagSet("daemon stop", flag.ExitOnError)
		root := fs.String("root", ".", "")
		kill := fs.Bool("kill", false, "")
		_ = fs.Parse(args[1:])
		return commands.DaemonStop(ctx, *root, commands.DaemonStopOptions{Kill: *kill})
	}
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	root := fs.String("root", ".", "")
	addr := fs.String("addr", "127.0.0.1:0", "")
	parentPID := fs.Int("parent-pid", 0, "")
	token := fs.String("token", "", "")
	startedBy := fs.String("started-by", "", "")
	_ = fs.Parse(args)
	return ueapi.Serve(ctx, ueapi.ServerOptions{Root: *root, Addr: *addr, ParentPID: *parentPID, Token: *token, StartedBy: *startedBy})
}

func runUEPlugin(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gamedepot ue-plugin <install|verify|diagnose|list|write-ubt-config>")
	}
	switch args[0] {
	case "install":
		fs := flag.NewFlagSet("ue-plugin install", flag.ExitOnError)
		project := fs.String("project", ".", "")
		overwrite := fs.Bool("overwrite", true, "")
		writeUBT := fs.Bool("write-ubt-config", false, "")
		_ = fs.Parse(args[1:])
		return ueplugin.Install(ueplugin.InstallOptions{Project: *project, Overwrite: *overwrite, EnablePlugin: true, WriteProjectConfig: true, WriteUBT: *writeUBT, VerifyAfter: true})
	case "verify":
		fs := flag.NewFlagSet("ue-plugin verify", flag.ExitOnError)
		project := fs.String("project", ".", "")
		_ = fs.Parse(args[1:])
		return ueplugin.Verify(*project)
	case "diagnose":
		fs := flag.NewFlagSet("ue-plugin diagnose", flag.ExitOnError)
		project := fs.String("project", ".", "")
		_ = fs.Parse(args[1:])
		return ueplugin.Diagnose(*project)
	case "list":
		fs := flag.NewFlagSet("ue-plugin list", flag.ExitOnError)
		project := fs.String("project", ".", "")
		_ = fs.Parse(args[1:])
		return ueplugin.List(*project)
	case "write-ubt-config":
		fs := flag.NewFlagSet("ue-plugin write-ubt-config", flag.ExitOnError)
		project := fs.String("project", ".", "")
		low := fs.Bool("low-memory", true, "")
		_ = fs.Parse(args[1:])
		return ueplugin.WriteUBTConfig(*project, *low)
	default:
		return fmt.Errorf("unknown ue-plugin subcommand %q", args[0])
	}
}

func printUsage() {
	fmt.Printf(`GameDepot %s

Core:
  gamedepot init [.] [--project <id>] [--remote <url>] [--branch <branch>] [--no-plugin]
  gamedepot clone [--branch <branch>] <remote-url> [dir]
  gamedepot status [--json]
  gamedepot update [--force|--strict]
  gamedepot conflicts [--json]
  gamedepot resolve <Content/path> (--remote|--local)
  gamedepot publish -m <message> [--dry-run]
  gamedepot history <Content/path> [--json]
  gamedepot checkout <ref> [--force]
  gamedepot doctor
  gamedepot verify
  gamedepot smoke-test [--workspace <dir>] [--clean] [--keep]

Config:
  gamedepot config path
  gamedepot config user [--name <name>] [--email <email>]
  gamedepot config profiles
  gamedepot config add-local <name> [--path <path>]
  gamedepot config add-oss <name> --bucket <bucket> [--region <region>]
  gamedepot config add-s3 <name> --endpoint <url> --bucket <bucket>
  gamedepot config set-credentials <profile> --access-key-id <id> --access-key-secret <secret>
  gamedepot config use <profile>
  gamedepot config project-use <profile>

Assets:
  gamedepot asset status [path] [--recursive] [--json]
  gamedepot asset history <path>
  gamedepot asset restore <path> --commit <commit> [--force]
  gamedepot asset revert <path>... [--force]


Storage:
  gamedepot store info
  gamedepot store check

UE:
  gamedepot ue-plugin install --project <UEProject> [--overwrite]
  gamedepot ue-plugin verify --project <UEProject>
  gamedepot ue-plugin diagnose --project <UEProject>
`, version)
}
