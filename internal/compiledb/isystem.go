package compiledb

import (
	"os"
	"os/exec"
	"strings"
)

func discoverImplicitIncludeDirs(compilerPath string) []string {
	if _, err := os.Stat(compilerPath); err != nil {
		return nil
	}

	cmd := exec.Command(compilerPath, "-E", "-x", "c++", "-v", "-")
	cmd.Stdin = strings.NewReader("")
	out, _ := cmd.CombinedOutput()
	return parseImplicitIncludeDirs(string(out))
}

func parseImplicitIncludeDirs(output string) []string {
	var dirs []string
	inList := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#include <...> search starts here:"):
			inList = true
		case trimmed == "End of search list.":
			inList = false
		case inList && trimmed != "":
			dirs = append(dirs, trimmed)
		}
	}
	return dirs
}
