package commands

import (
	"context"
	"fmt"
)

func GitConfigShow(ctx context.Context, start string) error {
	return fmt.Errorf("Git remotes are managed by Git; use `git remote`")
}
func GitConfigSetRemote(ctx context.Context, start, name, url string) error {
	return fmt.Errorf("Git remotes are managed by Git; use `git remote add/set-url`")
}
func GitConfigSetUpstream(ctx context.Context, start, name, url string) error {
	return fmt.Errorf("Git upstream is managed by Git")
}
func GitConfigTest(ctx context.Context, start string) error {
	return fmt.Errorf("Git remotes are managed by Git")
}
