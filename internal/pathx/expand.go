// Package pathx expands explicit relative path arguments into absolute paths.
package pathx

import (
	"path/filepath"
	"strings"
)

// ExpandArgs expands every explicit relative path in args. Non-path arguments
// are returned unchanged.
func ExpandArgs(cwd, home string, args []string) []string {
	if len(args) == 0 {
		return args
	}
	expanded := make([]string, len(args))
	for i, arg := range args {
		expanded[i] = Expand(cwd, home, arg)
	}
	return expanded
}

// Expand turns an explicit relative path into an absolute one. Only
// unambiguous forms are expanded: ".", "..", "./x", "../x", "~/x" and "~".
// Absolute paths, flags and opaque values are returned unchanged. When the
// base (cwd or home) is unknown the argument is returned unchanged.
func Expand(cwd, home, arg string) string {
	switch {
	case arg == "~":
		if home == "" {
			return arg
		}
		return home
	case strings.HasPrefix(arg, "~/"):
		if home == "" {
			return arg
		}
		return filepath.Join(home, arg[2:])
	case arg == "." || arg == ".." || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../"):
		if cwd == "" {
			return arg
		}
		return filepath.Join(cwd, arg)
	default:
		return arg
	}
}
