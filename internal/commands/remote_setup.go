package commands

import (
	"fmt"
	"strings"

	gdgit "github.com/1051093860/gamedepot/internal/git"
)

type GitRemoteSetupOptions struct {
	RemoteURL  string
	RemoteName string
	Branch     string
}

func normalizeRemoteName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "origin"
	}
	return name
}

// SetupGitRemoteAndBranch configures the local Git branch and remote without
// requiring a successful first push. It is safe for newly initialized empty
// repositories and existing repositories where the caller explicitly supplied a
// branch/remote.
func SetupGitRemoteAndBranch(root string, opts GitRemoteSetupOptions) error {
	g := gdgit.New(root)
	branch := strings.TrimSpace(opts.Branch)
	remoteURL := strings.TrimSpace(opts.RemoteURL)
	remoteName := normalizeRemoteName(opts.RemoteName)

	if branch != "" {
		if _, err := g.Run("branch", "-M", branch); err != nil {
			// Empty/unborn repositories can be awkward across Git versions.
			// symbolic-ref sets the initial branch without requiring a commit.
			if _, err2 := g.Run("symbolic-ref", "HEAD", "refs/heads/"+branch); err2 != nil {
				return fmt.Errorf("set git branch %q failed: %w; fallback failed: %v", branch, err, err2)
			}
		}
	} else if current, err := g.CurrentBranch(); err == nil && current != "" && current != "HEAD" {
		branch = current
	}

	if remoteURL != "" {
		if err := g.SetRemoteURL(remoteName, remoteURL); err != nil {
			return err
		}
	}

	if branch != "" && (remoteURL != "" || g.HasRemote(remoteName)) {
		// Configure upstream metadata before the first push. The remote branch
		// may not exist yet, which is fine; publish still uses git push -u.
		if _, err := g.Run("config", "branch."+branch+".remote", remoteName); err != nil {
			return err
		}
		if _, err := g.Run("config", "branch."+branch+".merge", "refs/heads/"+branch); err != nil {
			return err
		}
	}
	return nil
}
