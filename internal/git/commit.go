package git

func (g Git) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := g.Run(args...)
	return err
}

func (g Git) AddA(paths ...string) error {
	args := []string{"add", "-A", "--"}
	args = append(args, paths...)
	_, err := g.Run(args...)
	return err
}

func (g Git) Commit(message string) error {
	_, err := g.Run("commit", "-m", message)
	return err
}

func (g Git) StatusPorcelain(paths ...string) (string, error) {
	args := []string{"status", "--porcelain"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	return g.Run(args...)
}

func (g Git) StagedNames() (string, error) {
	return g.Run("diff", "--cached", "--name-only")
}

func (g Git) AddForce(paths ...string) error {
	args := []string{"add", "-f", "--"}
	args = append(args, paths...)
	_, err := g.Run(args...)
	return err
}

func (g Git) AddUpdate(paths ...string) error {
	args := []string{"add", "-A", "--"}
	args = append(args, paths...)
	_, err := g.Run(args...)
	return err
}

func (g Git) RmCached(paths ...string) error {
	args := []string{"rm", "--cached", "-f", "--"}
	args = append(args, paths...)
	_, err := g.Run(args...)
	return err
}
