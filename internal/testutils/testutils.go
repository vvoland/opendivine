// SPDX-License-Identifier: GPL-3.0-only

// Package testutils holds the small helpers the test suites share.
package testutils

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGameData returns the path to a Divine Divinity install, or skips the
// test when none is available: TEST_GAMEDATA_PATH first, then ./gamedata.
//
// The path has to BE an install, not merely exist: internal/game has a package
// directory called gamedata, and the fallback resolves against it.
func TestGameData(t testing.TB) string {
	t.Helper()
	for _, p := range []string{os.Getenv("TEST_GAMEDATA_PATH"), "./gamedata"} {
		if isInstall(p) {
			return p
		}
	}
	t.Skip("SKIP: no game data (set TEST_GAMEDATA_PATH, or put an install at ./gamedata)")
	return ""
}

// isInstall reports whether p is a directory holding the game binary.
func isInstall(p string) bool {
	if p == "" {
		return false
	}
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		return false
	}
	_, err := os.Stat(filepath.Join(p, "div.exe"))
	return err == nil
}
