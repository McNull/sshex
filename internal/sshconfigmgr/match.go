package sshconfigmgr

import (
	"strings"

	"github.com/mcnull/sshex/internal/sshx"
)

// matchCriterion reports whether token is an OpenSSH Match criterion keyword.
func matchCriterion(token string) bool {
	switch strings.ToLower(token) {
	case "all", "canonical", "exec", "final", "host", "localnetwork",
		"localuser", "originalhost", "tagged", "user":
		return true
	}
	return false
}

// normalizeMatchValue rewrites the criteria of a Match directive into the
// subset the ssh_config decoder understands. It returns the rewritten value
// and whether it differs from the original.
//
// "final" marks a block evaluated in the last OpenSSH pass; it is always true
// here, so it is dropped. Blocks whose remaining criteria cannot be evaluated
// (user, exec, originalhost, ...) are rewritten to "host !*", which parses but
// never matches, keeping their options inert instead of failing the file.
func normalizeMatchValue(value string) (string, bool) {
	original := strings.TrimSpace(value)
	args, err := sshx.SplitArgs(original)
	if err != nil {
		return original, false
	}
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.EqualFold(arg, "final") {
			continue
		}
		filtered = append(filtered, arg)
	}
	normalized := "host !*"
	switch {
	case len(filtered) == 0:
		normalized = "all"
	case len(filtered) == 1 && strings.EqualFold(filtered[0], "all"):
		normalized = "all"
	case strings.EqualFold(filtered[0], "host") && len(filtered) > 1 && !hasCriterion(filtered[1:]):
		normalized = "host " + strings.Join(filtered[1:], " ")
	}
	return normalized, normalized != original
}

// hasCriterion reports whether any token is a Match criterion keyword. It is
// used to detect combined directives such as "Match host a user b", which the
// decoder would otherwise misread as host patterns.
func hasCriterion(args []string) bool {
	for _, arg := range args {
		if matchCriterion(arg) {
			return true
		}
	}
	return false
}

// normalizeMatchDirectives rewrites Match directives in a flattened config so
// that one unsupported system file cannot abort host listing.
func normalizeMatchDirectives(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(stripComment(line))
		key, value, ok := cutDirective(trimmed)
		if !ok || !strings.EqualFold(key, "Match") {
			continue
		}
		normalized, changed := normalizeMatchValue(value)
		if !changed {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + "Match " + normalized
	}
	return []byte(strings.Join(lines, "\n"))
}

// cutDirective splits a directive line into its keyword and remaining value,
// accepting a space or tab as the separator.
func cutDirective(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, " ")
	if !ok {
		key, value, ok = strings.Cut(line, "\t")
	}
	return key, strings.TrimSpace(value), ok
}
