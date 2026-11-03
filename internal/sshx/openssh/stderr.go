package openssh

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

const sharedConnectionPrefix = "Shared connection to "

// sharedConnectionWriter rewrites the line OpenSSH prints when a multiplexed
// session exits. OpenSSH uses the resolved HostName in that message, while
// sshex names targets by the alias the user connected with. Rewriting also
// attributes the message to sshex.
//
// Writes are buffered until a newline so a message split across writes is still
// recognized; everything else passes through unchanged.
type sharedConnectionWriter struct {
	out     io.Writer
	label   string
	pending []byte
}

func newSharedConnectionWriter(out io.Writer, label string) *sharedConnectionWriter {
	if out == nil {
		out = io.Discard
	}
	return &sharedConnectionWriter{out: out, label: label}
}

func (w *sharedConnectionWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		idx := bytes.IndexByte(w.pending, '\n')
		if idx < 0 {
			break
		}
		line := w.pending[:idx+1]
		w.pending = w.pending[idx+1:]
		if err := w.writeLine(line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush writes any incomplete trailing line that was still buffered when the
// SSH client exited.
func (w *sharedConnectionWriter) Flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := w.pending
	w.pending = nil
	_, err := w.out.Write(line)
	return err
}

func (w *sharedConnectionWriter) writeLine(line []byte) error {
	trimmed := strings.TrimRight(string(line), "\r\n")
	if strings.HasPrefix(trimmed, sharedConnectionPrefix) && strings.HasSuffix(trimmed, " closed.") {
		_, err := fmt.Fprintf(w.out, "sshex: shared connection to %s closed.\n", w.label)
		return err
	}
	_, err := w.out.Write(line)
	return err
}
