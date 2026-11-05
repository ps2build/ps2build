package buildrun

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"

	"github.com/ps2build/ps2build/internal/utils"
)

// Result is the outcome of one ninja invocation.
type Result struct {
	// Success is true only if ninja exited 0.
	Success bool
	// Stdout/Stderr are ninja's own output, verbatim - the caller decides
	// whether/how to parse diagnostics out of it (see ParseDiagnostics).
	Stdout string
	Stderr string
	// Err is the error from starting or running ninja, nil on success.
	Err error
}

func FindNinja() (string, error) {
	ninjaPath := utils.SdkToolPath(utils.ExeSuffix("ninja"))

	if utils.FileExists(ninjaPath) {
		return ninjaPath, nil
	}

	return "", fmt.Errorf("ninja component not installed")
}

func Run(selfExe, outDir string, stdout, stderr io.Writer) (Result, error) {
	ninjaPath, err := FindNinja()
	if err != nil {
		return Result{}, err
	}

	var outBuf, errBuf bytes.Buffer
	outW := io.Writer(&outBuf)
	if stdout != nil {
		outW = io.MultiWriter(&outBuf, stdout)
	}
	errW := io.Writer(&errBuf)
	if stderr != nil {
		errW = io.MultiWriter(&errBuf, stderr)
	}

	cmd := exec.Command(ninjaPath, "-C", outDir)
	cmd.Stdout = outW
	cmd.Stderr = errW
	runErr := cmd.Run()

	return Result{
		Success: runErr == nil,
		Stdout:  outBuf.String(),
		Stderr:  errBuf.String(),
		Err:     runErr,
	}, nil
}
