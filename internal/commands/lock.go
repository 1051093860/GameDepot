package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/locks"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type LockOptions struct {
	Owner        string
	Host         string
	Note         string
	Force        bool
	AllowNonBlob bool
}

func Lock(ctx context.Context, start string, targetPath string, opts LockOptions) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	targetPath, err = workspace.CleanRelPath(targetPath)
	if err != nil {
		return err
	}
	if !opts.AllowNonBlob {
		class, err := workspace.ClassifyRel(targetPath, a.Config)
		if err != nil {
			return err
		}
		if class.Mode != rules.ModeBlob {
			return fmt.Errorf("lock is intended for blob-managed assets; %s is %s-managed. Use --allow-non-blob to lock anyway", targetPath, class.Mode)
		}
	}

	id := locks.DefaultIdentity()
	if opts.Owner != "" {
		id.Owner = opts.Owner
	}
	if opts.Host != "" {
		id.Host = opts.Host
	}

	mgr := locks.NewManager(a.Config.ProjectID, a.Store)
	entry, replaced, err := mgr.Lock(ctx, targetPath, id, opts.Note, opts.Force)
	if err != nil {
		return err
	}
	if replaced {
		fmt.Printf("Replaced lock: %s by %s@%s\n", entry.Path, entry.Owner, entry.Host)
	} else {
		fmt.Printf("Locked: %s by %s@%s\n", entry.Path, entry.Owner, entry.Host)
	}
	return nil
}

func Unlock(ctx context.Context, start string, targetPath string, owner string, host string, force bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	targetPath, err = workspace.CleanRelPath(targetPath)
	if err != nil {
		return err
	}

	id := locks.DefaultIdentity()
	if owner != "" {
		id.Owner = owner
	}
	if host != "" {
		id.Host = host
	}

	mgr := locks.NewManager(a.Config.ProjectID, a.Store)
	entry, err := mgr.Unlock(ctx, targetPath, id, force)
	if err != nil {
		return err
	}
	fmt.Printf("Unlocked: %s previously by %s@%s\n", entry.Path, entry.Owner, entry.Host)
	return nil
}

func Locks(ctx context.Context, start string, jsonOut bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	mgr := locks.NewManager(a.Config.ProjectID, a.Store)
	entries, err := mgr.List(ctx)
	if err != nil {
		return err
	}
	if jsonOut {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if len(entries) == 0 {
		fmt.Println("No locks")
		return nil
	}
	fmt.Println("path                                      owner                 host                  created_at")
	for _, e := range entries {
		fmt.Printf("%-40s  %-20s  %-20s  %s\n", e.Path, e.Owner, e.Host, e.CreatedAt)
		if e.Note != "" {
			fmt.Printf("  note: %s\n", e.Note)
		}
	}
	return nil
}
