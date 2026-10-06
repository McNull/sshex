package service

import (
	"strconv"
	"strings"
)

// RenderCommand expands the supported shell parameter forms in a command
// template so the CLI can show what is about to run. It is a best-effort
// mirror of the origin shell and is only used for display; execution always
// goes through the shell.
//
// Supported: ${VAR}, $VAR, ${VAR:-default}, $@, ${@}, ${@:-default}, $#,
// $0..$9 and braced ${10}. Unknown variables render empty.
func RenderCommand(template string, env []string, name string, args []string) string {
	return render(template, envMap(env), name, args)
}

func render(template string, values map[string]string, name string, args []string) string {
	var b strings.Builder
	for i := 0; i < len(template); {
		if template[i] != '$' {
			b.WriteByte(template[i])
			i++
			continue
		}
		if i+1 >= len(template) {
			b.WriteByte('$')
			break
		}
		next := template[i+1]
		switch {
		case next == '{':
			end := matchingBrace(template, i+1)
			if end < 0 {
				b.WriteByte('$')
				i++
				continue
			}
			b.WriteString(expandBraced(template[i+2:end], values, name, args))
			i = end + 1
		case next == '@':
			b.WriteString(strings.Join(args, " "))
			i += 2
		case next == '#':
			b.WriteString(strconv.Itoa(len(args)))
			i += 2
		case next >= '0' && next <= '9':
			b.WriteString(positional(int(next-'0'), name, args))
			i += 2
		case isNameStart(next):
			j := i + 1
			for j < len(template) && isNameChar(template[j]) {
				j++
			}
			b.WriteString(values[template[i+1:j]])
			i = j
		default:
			b.WriteByte('$')
			i++
		}
	}
	return b.String()
}

func expandBraced(content string, values map[string]string, name string, args []string) string {
	key, def, hasDef := splitDefault(content)
	value := resolve(key, values, name, args)
	if value == "" && hasDef {
		return render(def, values, name, args)
	}
	return value
}

func resolve(key string, values map[string]string, name string, args []string) string {
	switch key {
	case "":
		return ""
	case "@":
		return strings.Join(args, " ")
	case "#":
		return strconv.Itoa(len(args))
	}
	if n, err := strconv.Atoi(key); err == nil {
		return positional(n, name, args)
	}
	return values[key]
}

func positional(n int, name string, args []string) string {
	if n == 0 {
		return name
	}
	if n > len(args) {
		return ""
	}
	return args[n-1]
}

// splitDefault splits a braced expression into its name and default at the
// first top-level ":-" or "-", skipping nested ${...}.
func splitDefault(content string) (name, def string, hasDef bool) {
	depth := 0
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ':':
			if depth == 0 && i+1 < len(content) && content[i+1] == '-' {
				return content[:i], content[i+2:], true
			}
		case '-':
			if depth == 0 {
				return content[:i], content[i+1:], true
			}
		}
	}
	return content, "", false
}

func matchingBrace(template string, open int) int {
	depth := 0
	for i := open; i < len(template); i++ {
		switch template[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func envMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			values[entry[:idx]] = entry[idx+1:]
		}
	}
	return values
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
