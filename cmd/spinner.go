package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// spinnerFrames is the braille cycle drawn while work is in progress.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerInterval is the delay between animation frames.
const spinnerInterval = 80 * time.Millisecond

// isTerminalWriter reports whether w is backed by a terminal device. It is a
// variable so tests can force the animated path without a real terminal.
var isTerminalWriter = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// spinner animates a progress indicator in front of a label while work runs.
// When the output is not a terminal it is inert and prints nothing, so pipes
// and tests are unaffected.
type spinner struct {
	out   io.Writer
	label string
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

// startSpinner begins animating label in front of a braille frame. Callers must
// eventually call Stop or StopWith to release the animation goroutine.
func startSpinner(out io.Writer, label string) *spinner {
	s := &spinner{out: out, label: label}
	if !isTerminalWriter(out) {
		return s
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.run()
	return s
}

func (s *spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()
	for i := 0; ; {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			fmt.Fprintf(s.out, "\r%s %s", spinnerFrames[i], s.label)
			i = (i + 1) % len(spinnerFrames)
		}
	}
}

// Stop halts the animation and erases the line. It is safe to call more than
// once.
func (s *spinner) Stop() {
	s.finish("")
}

// StopWith halts the animation and replaces the line with msg followed by a
// newline.
func (s *spinner) StopWith(msg string) {
	s.finish(msg)
}

func (s *spinner) finish(msg string) {
	if s.stop == nil {
		return
	}
	s.once.Do(func() {
		close(s.stop)
		<-s.done
		if msg == "" {
			fmt.Fprint(s.out, "\r\x1b[K")
			return
		}
		fmt.Fprintf(s.out, "\r\x1b[K%s\n", msg)
	})
}
