package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// replacePath atomic/overwriting replaces targetPath with srcPath.
// It works across filesystem boundaries and for both files and directories.
func replacePath(targetPath, srcPath string) error {
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source path %s: %w", srcPath, err)
	}

	if srcInfo.IsDir() {
		return replaceDir(targetPath, srcPath)
	}

	return replaceFile(targetPath, srcPath)
}

// replaceFile moves/copies a single file from src to target, overwriting target if present.
func replaceFile(targetPath, srcPath string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	// Try atomic fast-path rename
	err := os.Rename(srcPath, targetPath)
	if err == nil {
		return nil
	}

	// Fallback for cross-device moves or Windows locks: copy and delete source
	if err := copyFileContents(srcPath, targetPath); err != nil {
		return fmt.Errorf("copying file %s to %s: %w", srcPath, targetPath, err)
	}

	return os.Remove(srcPath)
}

// replaceDir moves/copies all contents from srcDir into targetDir, overwriting existing files.
func replaceDir(targetDir, srcDir string) error {
	// Try atomic fast-path rename
	err := os.Rename(srcDir, targetDir)
	if err == nil {
		return nil
	}

	// Fallback: Recursive copy & remove for cross-device directories
	if err := copyDirContents(srcDir, targetDir); err != nil {
		return err
	}

	return os.RemoveAll(srcDir)
}

// copyDirContents recursively copies all entries from src to target.
func copyDirContents(src, target string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("reading dir %s: %w", src, err)
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		targetPath := filepath.Join(target, entry.Name())

		if entry.IsDir() {
			if err := copyDirContents(srcPath, targetPath); err != nil {
				return err
			}
		} else {
			if err := copyFileContents(srcPath, targetPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFileContents copies file permissions and raw bytes from src to dst.
func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	srcStat, err := in.Stat()
	if err != nil {
		return err
	}

	// Remove target first to prevent permission/lock errors on Windows
	_ = os.Remove(dst)

	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcStat.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}
