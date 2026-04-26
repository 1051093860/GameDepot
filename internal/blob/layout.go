package blob

import "path/filepath"

func PathForSHA256(root string, sha string) string {
	if len(sha) < 4 {
		return filepath.Join(root, "sha256", sha+".blob")
	}

	return filepath.Join(
		root,
		"sha256",
		sha[0:2],
		sha[2:4],
		sha+".blob",
	)
}
