package git

import (
	"strconv"
	"strings"
)

type CommitInfo struct {
	Hash    string
	Time    string
	Subject string
}

func (g Git) LogFile(path string) ([]CommitInfo, error) {
	out, err := g.Run("log", "--format=%H|%ci|%s", "--", path)
	if err != nil {
		return nil, err
	}
	return parseCommitInfo(out), nil
}

func (g Git) RevList(args ...string) ([]string, error) {
	if len(args) == 0 {
		args = []string{"--date-order", "HEAD"}
	}
	out, err := g.Run(append([]string{"rev-list"}, args...)...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func (g Git) CommitInfo(hash string) (CommitInfo, error) {
	out, err := g.Run("show", "-s", "--format=%H|%ci|%s", hash)
	if err != nil {
		return CommitInfo{}, err
	}
	items := parseCommitInfo(out)
	if len(items) == 0 {
		return CommitInfo{Hash: hash}, nil
	}
	return items[0], nil
}

func (g Git) ShowFileAtCommit(commit string, path string) (string, error) {
	b, err := g.ShowFileBytes(commit, path)
	return string(b), err
}

func (g Git) ShowFileBytes(commit string, path string) ([]byte, error) {
	return g.RunBytes("show", commit+":"+path)
}

func (g Git) CatFileSize(commit string, path string) (int64, error) {
	out, err := g.Run("cat-file", "-s", commit+":"+path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(out), 10, 64)
}

func (g Git) FileExistsAtCommit(commit string, path string) bool {
	_, err := g.Run("cat-file", "-e", commit+":"+path)
	return err == nil
}

func (g Git) CheckoutFile(path string) error {
	_, err := g.Run("checkout", "--", path)
	return err
}

func (g Git) CurrentCommit() (string, error) {
	out, err := g.Run("rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

func (g Git) Tags() ([]string, error) {
	out, err := g.Run("tag", "--list")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func parseCommitInfo(out string) []CommitInfo {
	var commits []CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		ci := CommitInfo{Hash: parts[0]}
		if len(parts) > 1 {
			ci.Time = parts[1]
		}
		if len(parts) > 2 {
			ci.Subject = parts[2]
		}
		commits = append(commits, ci)
	}
	return commits
}
