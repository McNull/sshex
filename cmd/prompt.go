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
	out    io.Writer
	errOut io.Writer
	// quiet suppresses field descriptions and blank separators, leaving just
	// the prompts for a clean recording.
	quiet bool
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
			return &prompter{editor: ed, out: out, errOut: errOut}
		}
	}
	return &prompter{editor: &plainEditor{in: bufio.NewReader(in), out: out}, out: out, errOut: errOut}
}

// maxFieldAttempts bounds how many times a single field is re-prompted after a
// validation failure, so a stream of invalid input terminates cleanly.
const maxFieldAttempts = 3

// field prompts for a string value and re-prompts the same field until the
// answer passes validate. The description is printed once, before the first
// prompt. An empty answer clears the value when the field is optional and is
// rejected when it is required. After a rejected answer the next prompt is
// pre-filled with what was attempted (empty if it was cleared) so the answer can
// be corrected without the previous value being appended to.
func (p *prompter) field(label, description, current string, required bool, validate func(string) error) (string, error) {
	p.describe(description)
	prefill := current
	for attempt := 0; ; attempt++ {
		value, err := p.editor.readLine(label, prefill)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" && !required {
			return "", nil
		}
		if err := validate(value); err != nil {
			if attempt+1 >= maxFieldAttempts {
				return "", err
			}
			p.reject(err)
			prefill = value
			continue
		}
		return value, nil
	}
}

// reject prints a validation message above the retried prompt without repeating
// the field description.
func (p *prompter) reject(err error) {
	if p.errOut == nil {
		return
	}
	fmt.Fprintln(p.errOut, err)
}

// boolean prompts for a yes/no value, keeping the current value on an empty
// line or an unrecognized answer.
func (p *prompter) boolean(label, description string, current bool) (bool, error) {
	p.describe(description)
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

// describe prints a field description above its prompt, one comment line per
// non-empty line of text. Blank lines are preserved as bare comment lines. When
// the output is a color terminal the description is dimmed so the editable
// prompt stays in focus; otherwise it is printed as plain text.
func (p *prompter) describe(description string) {
	if p.quiet || p.out == nil {
		return
	}
	color := isColorTerminal(p.out)
	for _, line := range strings.Split(strings.TrimRight(description, "\n"), "\n") {
		text := "#"
		if line != "" {
			text = "# " + line
		}
		fmt.Fprintln(p.out, dimmed(text, color))
	}
}

// separate writes a blank line so the descriptions and prompts of consecutive
// fields do not run together.
func (p *prompter) separate() {
	if p.quiet || p.out == nil {
		return
	}
	fmt.Fprintln(p.out)
}

// dimStart and dimEnd wrap descriptions in faint bright black. Bright black
// alone renders identically to the default foreground in some color themes, so
// the faint attribute is added to guarantee the text is dimmer than the prompt;
// every line is reset so the color never leaks into the readline prompt.
const (
	dimStart = "\x1b[2;92m"
	dimEnd   = "\x1b[0m"
)

// dimmed wraps line in the dim color when color is true.
func dimmed(line string, color bool) string {
	if !color {
		return line
	}
	return dimStart + line + dimEnd
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
	fmt.Fprintln(e.out)
	return strings.TrimSpace(line), nil
}

func (e *plainEditor) close() error {
	return nil
}
