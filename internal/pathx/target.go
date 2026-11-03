package pathx

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// KindFolder marks an exec target that is an existing directory.
	KindFolder = "folder"
	// KindFile marks an exec target that is a file or does not exist yet.
	KindFile = "file"
)

// ResolveTarget resolves the target argument of an exec invocation. With no
// arguments it returns cwd. The target is expanded like an explicit relative
// path and then made absolute against cwd, so a bare relative name works too.
// Flags are returned unchanged.
func ResolveTarget(cwd, home string, args []string) string {
	if len(args) == 0 {
		return cwd
	}
	arg := Expand(cwd, home, args[0])
	if arg == "" || strings.HasPrefix(arg, "-") {
		return arg
	}
	if filepath.IsAbs(arg) {
		return filepath.Clean(arg)
	}
	if cwd == "" {
		return arg
	}
	return filepath.Join(cwd, arg)
}

// ClassifyTarget reports whether path names an existing directory. Anything
// else, including a missing path, is treated as a file.
func ClassifyTarget(path string) string {
	if path != "" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return KindFolder
		}
	}
	return KindFile
}
