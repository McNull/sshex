package sshx

import "fmt"

// SplitArgs splits s into arguments the way a POSIX shell would for a simple
// command line. Single quotes are literal, double quotes honor backslash
// escapes, and a backslash escapes the next character outside quotes. No
// expansion is performed; the result is meant for direct use with
// exec.Command.
func SplitArgs(s string) ([]string, error) {
	const (
		normal = iota
		single
		double
	)

	var args []string
	var cur []rune
	started := false
	state := normal
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch state {
		case single:
			if r == '\'' {
				state = normal
				continue
			}
			cur = append(cur, r)
		case double:
			switch r {
			case '"':
				state = normal
			case '\\':
				if i+1 >= len(runes) {
					return nil, fmt.Errorf("unterminated escape in %q", s)
				}
				i++
				cur = append(cur, runes[i])
			default:
				cur = append(cur, r)
			}
		default:
			switch {
			case r == ' ' || r == '\t' || r == '\n' || r == '\r':
				if started {
					args = append(args, string(cur))
					cur = cur[:0]
					started = false
				}
			case r == '\'':
				state = single
				started = true
			case r == '"':
				state = double
				started = true
			case r == '\\':
				if i+1 >= len(runes) {
					return nil, fmt.Errorf("unterminated escape in %q", s)
				}
				i++
				cur = append(cur, runes[i])
				started = true
			default:
				cur = append(cur, r)
				started = true
			}
		}
	}

	if state != normal {
		return nil, fmt.Errorf("unterminated quote in %q", s)
	}
	if started {
		args = append(args, string(cur))
	}
	return args, nil
}
