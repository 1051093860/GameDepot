package git

import "strings"

type CommitInfo struct {
	Hash string
	Time string
}

func (g Git) LogFile(path string) ([]CommitInfo, error) {
	out, err := g.Run("log", "--format=%H|%ci", "--", path)
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}

		commits = append(commits, CommitInfo{
			Hash: parts[0],
			Time: parts[1],
		})
	}

	return commits, nil
}

func (g Git) ShowFileAtCommit(commit string, path string) (string, error) {
	return g.Run("show", commit+":"+path)
}

func (g Git) Tags() ([]string, error) {
	out, err := g.Run("tag", "--list")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}
