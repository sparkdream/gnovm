package keeper

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed stdlibs
var embeddedStdlibs embed.FS

// extractEmbeddedStdlibs writes the embedded stdlib files to a temporary
// directory and returns its path. The caller should defer os.RemoveAll on
// the returned path once LoadStdlib has finished.
func extractEmbeddedStdlibs() (string, error) {
	tmpDir, err := os.MkdirTemp("", "gnovm-stdlibs-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for gno stdlibs: %w", err)
	}

	err = fs.WalkDir(embeddedStdlibs, "stdlibs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Strip the leading "stdlibs/" prefix so the temp dir structure
		// matches what LoadStdlib expects (e.g. tmpDir/errors/errors.gno).
		relPath, err := filepath.Rel("stdlibs", path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(tmpDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		data, err := embeddedStdlibs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}
		return os.WriteFile(targetPath, data, 0o644)
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to extract embedded gno stdlibs: %w", err)
	}

	return tmpDir, nil
}
