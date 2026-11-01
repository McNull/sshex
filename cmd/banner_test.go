package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestStripANSI(t *testing.T) {
	got := stripANSI("\x1b[0;97mhello\x1b[0m world")
	if got != "hello world" {
		t.Fatalf("stripANSI() = %q, want %q", got, "hello world")
	}
}

func TestPrintBanner(t *testing.T) {
	var buf bytes.Buffer
	printBanner(&buf)

	out := buf.String()
	if !strings.Contains(out, " v"+version) {
		t.Fatalf("banner missing version, got %q", out)
	}
	if strings.Contains(out, "sshex "+version) {
		t.Errorf("banner still contains 'sshex' text: %q", out)
	}
	if strings.Contains(out, "{VERSION}") {
		t.Errorf("banner still contains {VERSION} placeholder: %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("banner contains ANSI escapes for non-terminal writer: %q", out)
	}
}

func TestShowBanner(t *testing.T) {
	if !showBanner(rootCmd) {
		t.Error("showBanner(rootCmd) = false, want true")
	}

	annotated := &cobra.Command{Annotations: map[string]string{bannerAnnotation: "true"}}
	if !showBanner(annotated) {
		t.Error("showBanner(annotated) = false, want true")
	}

	plain := &cobra.Command{}
	if showBanner(plain) {
		t.Error("showBanner(plain) = true, want false")
	}
}

func TestShowBannerDemoMode(t *testing.T) {
	orig := recordDemoMode
	recordDemoMode = func() bool { return true }
	t.Cleanup(func() { recordDemoMode = orig })

	if showBanner(rootCmd) {
		t.Error("showBanner(rootCmd) = true in demo mode, want false")
	}
	annotated := &cobra.Command{Annotations: map[string]string{bannerAnnotation: "true"}}
	if showBanner(annotated) {
		t.Error("showBanner(annotated) = true in demo mode, want false")
	}
}
