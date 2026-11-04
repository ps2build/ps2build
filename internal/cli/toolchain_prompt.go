package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ps2build/ps2build/internal/buildgraph"
	"github.com/ps2build/ps2build/internal/update"
	"github.com/ps2build/ps2build/internal/utils"
)

// offerToolchainInstall handles a build that failed because its toolchain is missing: on a terminal it offers
// to install it and reports whether the caller should retry.
func offerToolchainInstall(err error, in io.Reader, interactive bool, out io.Writer) bool {
	var missing *buildgraph.NotInstalledError
	if !errors.As(err, &missing) || !interactive {
		return false
	}

	fmt.Fprintf(out, "The %s toolchain is not installed. Install it now? [Y/n] ", missing.Kind)
	answer, _ := bufio.NewReader(in).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "" && a != "y" && a != "yes" {
		return false
	}

	if _, err := update.Run(update.Options{SDKRoot: utils.DetectSDKRoot(), Component: missing.Component}); err != nil {
		fmt.Fprintf(out, "install failed: %v\n", err)
		return false
	}
	return true
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
