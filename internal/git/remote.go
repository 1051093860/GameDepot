package git

import "strings"

func (g Git) Fetch(remote string) error {
	if strings.TrimSpace(remote) == "" {
		remote = "origin"
	}
	_, err := g.Run("fetch", remote)
	return err
}

func (g Git) PullFFOnly(remote string, branch string) error {
	if strings.TrimSpace(remote) == "" {
		remote = "origin"
	}
	if strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	_, err := g.Run("pull", "--ff-only", remote, branch)
	return err
}

func (g Git) Push(remote string, branch string) error {
	if strings.TrimSpace(remote) == "" {
		remote = "origin"
	}
	if strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	_, err := g.Run("push", "-u", remote, branch)
	return err
}

func (g Git) RemoteURL(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = "origin"
	}
	out, err := g.Run("remote", "get-url", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g Git) HasRemote(name string) bool {
	_, err := g.RemoteURL(name)
	return err == nil
}

func (g Git) SetRemoteURL(name, url string) error {
	if strings.TrimSpace(name) == "" {
		name = "origin"
	}
	if g.HasRemote(name) {
		_, err := g.Run("remote", "set-url", name, url)
		return err
	}
	_, err := g.Run("remote", "add", name, url)
	return err
}

func (g Git) LsRemote(name string) error {
	if strings.TrimSpace(name) == "" {
		name = "origin"
	}
	_, err := g.Run("ls-remote", name)
	return err
}

func (g Git) CurrentBranch() (string, error) {
	out, err := g.Run("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	// In an unborn repository (no commits yet), rev-parse can fail because HEAD
	// has no object. symbolic-ref still knows the intended branch name.
	out, err2 := g.Run("symbolic-ref", "--short", "HEAD")
	if err2 == nil {
		return strings.TrimSpace(out), nil
	}
	return "", err
}

func (g Git) HasAnyRemote() bool {
	out, err := g.Run("remote")
	return err == nil && strings.TrimSpace(out) != ""
}
