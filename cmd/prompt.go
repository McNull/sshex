package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ergochat/readline"
)

// errEditCancelled signals that the user aborted an interactive edit (Ctrl+C).
var errEditCancelled = errors.New("edit cancelled")

// prompter reads field values for the interactive editors. When the input is a
// terminal the current value is pre-filled in an editable line; otherwise it
// falls back to a plain "[current]" prompt so pipes and tests keep working.
type prompter struct {
	editor editor
}

// editor abstracts reading a single line, either through a terminal line editor
// or a plain buffered reader.
type editor interface {
	// readLine presents label with an optional pre-filled defaultValue and
	// returns the edited line. It may return an empty line.
	readLine(label, defaultValue string) (string, error)
	close() error
}

func newPrompter(in io.Reader, out, errOut io.Writer) *prompter {
	if isTerminal(in) {
		if ed, err := newTerminalEditor(in, out, errOut); err == nil {
			return &prompter{editor: ed}
		}
	}
	return &prompter{editor: &plainEditor{in: bufio.NewReader(in), out: out}}
}

// field prompts for a string value. An empty answer keeps the current value.
func (p *prompter) field(label, current string) (string, error) {
	value, err := p.editor.readLine(label, current)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return current, nil
	}
	return value, nil
}

// boolean prompts for a yes/no value, keeping the current value on an empty
// line or an unrecognized answer.
func (p *prompter) boolean(label string, current bool) (bool, error) {
	shown := "no"
	if current {
		shown = "yes"
	}
	line, err := p.editor.readLine(label+" (yes/no)", shown)
	if err != nil {
		return current, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", shown:
		return current, nil
	case "y", "yes", "true":
		return true, nil
	case "n", "no", "false":
		return false, nil
	default:
		return current, nil
	}
}

func (p *prompter) close() error {
	return p.editor.close()
}

// isTerminal reports whether r is backed by a terminal device.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// terminalEditor reads lines with full inline editing: the current value is
// pre-filled and the cursor keys, home/end, word navigation and UTF-8 input all
// work.
type terminalEditor struct {
	rl *readline.Instance
}

func newTerminalEditor(in io.Reader, out, errOut io.Writer) (*terminalEditor, error) {
	rl, err := readline.NewFromConfig(&readline.Config{
		Stdin:                  in,
		Stdout:                 out,
		Stderr:                 errOut,
		HistoryLimit:           -1,
		DisableAutoSaveHistory: true,
		InterruptPrompt:        "^C",
		EOFPrompt:              "",
	})
	if err != nil {
		return nil, err
	}
	return &terminalEditor{rl: rl}, nil
}

func (e *terminalEditor) readLine(label, defaultValue string) (string, error) {
	e.rl.SetPrompt(label + ": ")
	line, err := e.rl.ReadLineWithDefault(defaultValue)
	if errors.Is(err, readline.ErrInterrupt) {
		return "", errEditCancelled
	}
	if err != nil {
		return "", err
	}
	return line, nil
}

func (e *terminalEditor) close() error {
	return e.rl.Close()
}

// plainEditor is the non-terminal fallback. It shows the current value in
// brackets and reads a line without any terminal manipulation.
type plainEditor struct {
	in  *bufio.Reader
	out io.Writer
}

func (e *plainEditor) readLine(label, defaultValue string) (string, error) {
	fmt.Fprintf(e.out, "%s [%s]: ", label, defaultValue)
	line, err := e.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (e *plainEditor) close() error {
	return nil
}
