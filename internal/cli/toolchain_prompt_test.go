package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

func missingLLVM() error {
	return fmt.Errorf("generating: %w", &buildgraph.NotInstalledError{Kind: buildgraph.ToolchainLLVM, Component: "llvm-toolchain", Root: "/x"})
}

func TestOfferToolchainInstallDoesNotRetry(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		input       string
		interactive bool
	}{
		{"not a terminal", missingLLVM(), "y\n", false},
		{"user declines", missingLLVM(), "n\n", true},
		{"unrelated error", errors.New("boom"), "y\n", true},
		{"no error", nil, "y\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if offerToolchainInstall(tc.err, strings.NewReader(tc.input), tc.interactive, &out) {
				t.Errorf("offerToolchainInstall() = true, want false (output: %q)", out.String())
			}
		})
	}
}

func TestOfferToolchainInstallAsksOnATerminal(t *testing.T) {
	var out bytes.Buffer
	offerToolchainInstall(missingLLVM(), strings.NewReader("n\n"), true, &out)
	if !strings.Contains(out.String(), "The llvm toolchain is not installed. Install it now? [Y/n]") {
		t.Errorf("prompt = %q, want the install question", out.String())
	}
}
