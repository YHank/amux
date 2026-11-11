package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Herdr exposes native terminal popups through manifest-declared entrypoints.
// Keep this manifest beside amux preferences; it always invokes this executable.
func writeHerdrPlugin() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "amux", "herdr-plugin")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	manifest := fmt.Sprintf(`id = "amux.dashboard"
name = "amux"
version = "1.0.0"
min_herdr_version = "0.9.5"

[[panes]]
id = "dashboard"
title = "amux"
command = [%s, "--backend", "herdr"]

[[panes]]
id = "terminal"
title = "amux agent"
command = [%s, "herdr-attach"]
`, strconv.Quote(self), strconv.Quote(self))
	if err := os.WriteFile(filepath.Join(path, "herdr-plugin.toml"), []byte(manifest), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
