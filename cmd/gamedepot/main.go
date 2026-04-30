package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/1051093860/gamedepot/internal/commands"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/ueapi"
	"github.com/1051093860/gamedepot/internal/ueplugin"
)

var version = "v0.10.0"

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
	case "doctor":
		return commands.Doctor(ctx, ".")
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
	case "submit":
		fs := flag.NewFlagSet("submit", flag.ExitOnError)
		msg := fs.String("m", "", "commit message")
		allow := fs.Bool("allow-unmanaged", false, "advanced: do not fail on review files")
		_ = fs.Parse(os.Args[2:])
		return commands.SubmitWithOptions(ctx, ".", *msg, commands.SubmitOptions{AllowUnmanaged: *allow})
	case "push":
		return commands.Push(ctx, ".")
	case "pull":
		return commands.Pull(ctx, ".")
	case "sync":
		fs := flag.NewFlagSet("sync", flag.ExitOnError)
		force := fs.Bool("force", false, "discard local unsubmitted blob changes")
		_ = fs.Parse(os.Args[2:])
		return commands.SyncWithOptions(ctx, ".", commands.SyncOptions{Force: *force})
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
	case "rules":
		return runRules(ctx, os.Args[2:])
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
	case "history":
		return runAsset(ctx, append([]string{"history"}, os.Args[2:]...))
	case "restore-version":
		return runAsset(ctx, append([]string{"restore"}, os.Args[2:]...))
	case "revert-assets":
		return runAsset(ctx, append([]string{"revert"}, os.Args[2:]...))
	case "restore":
		return runBlobRestore(ctx, os.Args[2:])
	case "classify", "list", "ls", "repair-current-blob", "git-config", "project", "smoke-test", "cmd-smoke-test", "ue-api-smoke-test", "v08-core-smoke-test", "v08-api-smoke-test", "ue-plugin-smoke-test", "remote-smoke-test", "team-smoke-test":
		return fmt.Errorf("%q has been removed or hidden in v0.8; use the structured commands shown by `gamedepot help`", os.Args[1])
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func runInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	projectID := fs.String("project", "", "project id; defaults to .uproject name")
	noPlugin := fs.Bool("no-plugin", false, "do not install the UE plugin")
	overwritePlugin := fs.Bool("overwrite-plugin", true, "overwrite existing GameDepotUE plugin")
	_ = fs.Parse(args)
	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}
	if err := commands.ProjectInitUE(ctx, root, commands.ProjectInitUEOptions{Project: *projectID, Profile: "local"}); err != nil {
		return err
	}
	if !*noPlugin {
		if err := ueplugin.Install(ueplugin.InstallOptions{Project: root, Overwrite: *overwritePlugin, EnablePlugin: true, WriteProjectConfig: true, VerifyAfter: true}); err != nil {
			return err
		}
	}
	return nil
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

func runRules(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gamedepot rules <list|set|delete|move|enable|disable>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("rules list", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "")
		_ = fs.Parse(args[1:])
		return commands.RulesList(ctx, ".", *jsonOut)
	case "set":
		fs := flag.NewFlagSet("rules set", flag.ExitOnError)
		mode := fs.String("mode", "", "git|blob|ignore")
		scope := fs.String("scope", "exact", "exact|directory|extension|glob")
		jsonOut := fs.Bool("json", false, "")
		_ = fs.Parse(args[1:])
		if *mode == "" || fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot rules set <pattern> --mode <git|blob|ignore> [--scope exact|directory|extension|glob]")
		}
		_, err := commands.RulesSet(ctx, ".", commands.RuleSetOptions{Paths: fs.Args(), Mode: rules.Mode(*mode), Scope: commands.RuleScope(*scope)}, *jsonOut)
		return err
	case "delete":
		if len(args) < 2 {
			return fmt.Errorf("usage: gamedepot rules delete <id-or-pattern>")
		}
		return commands.RulesDelete(ctx, ".", args[1])
	case "move":
		fs := flag.NewFlagSet("rules move", flag.ExitOnError)
		up := fs.Bool("up", false, "")
		down := fs.Bool("down", false, "")
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: gamedepot rules move <id-or-pattern> --up|--down")
		}
		delta := 0
		if *up {
			delta = -1
		}
		if *down {
			delta = 1
		}
		if delta == 0 {
			return fmt.Errorf("rules move requires --up or --down")
		}
		return commands.RulesMove(ctx, ".", fs.Arg(0), delta)
	case "enable":
		if len(args) < 2 {
			return fmt.Errorf("usage: gamedepot rules enable <id-or-pattern>")
		}
		return commands.RulesSetEnabled(ctx, ".", args[1], true)
	case "disable":
		if len(args) < 2 {
			return fmt.Errorf("usage: gamedepot rules disable <id-or-pattern>")
		}
		return commands.RulesSetEnabled(ctx, ".", args[1], false)
	default:
		return fmt.Errorf("unknown rules subcommand %q", args[0])
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
		if len(args) < 2 {
			return fmt.Errorf("usage: gamedepot asset history <path>")
		}
		return commands.History(ctx, ".", args[1])
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
  gamedepot init [.] [--project <id>] [--no-plugin]
  gamedepot status [--json]
  gamedepot submit -m <message>
  gamedepot push
  gamedepot pull
  gamedepot sync [--force]
  gamedepot checkout <ref> [--force]
  gamedepot doctor
  gamedepot verify

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

Rules:
  gamedepot rules list [--json]
  gamedepot rules set <pattern> --mode <git|blob|ignore> [--scope <exact|directory|extension|glob>]
  gamedepot rules delete <id-or-pattern>
  gamedepot rules move <id-or-pattern> --up|--down
  gamedepot rules enable <id-or-pattern>
  gamedepot rules disable <id-or-pattern>

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
