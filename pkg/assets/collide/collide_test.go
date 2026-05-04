// SPDX-License-Identifier: GPL-3.0-only

package collide

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grono.dev/opendivine/internal/testutils"
)

// Expected record counts per Collide.<n>; cross-checked against the
// CPacked imagelist entry counts where they're paired (matches confirm
// the "per-sprite collision-mask" interpretation).
var expected = []struct {
	name      string // on-disk filename under static/imagelists/
	count     int
	cpackedID int // -1 = not paired with a CPacked list
}{
	{"Collide.0", 7208, 0},
	{"Collide.1", 78853, 1},
	{"Collide.2", 0, -1},
	{"Collide.3", 1336, 3},
	{"Collide.4", 383, 4},
	{"Collide.5", 734, -1}, // 5 entries differ from CPacked.5's 262
	{"Collide.6", 55, -1},
	{"Collide.7", 4838, 7},
	{"Collide.8", 667, -1},
	{"Collide.9", 9, 9},
	{"Collide.10", 78266, -1},
	{"Collide.11", 131, 11},
	{"Collide.12", 1387, 12},
}

// TestAllCollideFiles loads every shipped Collide.<n> from
// $TEST_GAMEDATA_PATH/static/imagelists/ and validates record counts.
// Skips the whole table when the env var is unset; skips individual
// rows when that specific Collide.<n> isn't shipped (the Steam
// re-release, for example, only includes Collide.{2,3,5}).
//
// Filename casing varies by install (Collide vs COLLIDE), so we
// scan the directory once and match case-insensitively rather than
// hard-coding either form.
func TestAllCollideFiles(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	dir := filepath.Join(gamedata, "static/imagelists")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	available := map[string]string{} // lower-case name -> on-disk name
	for _, e := range entries {
		available[strings.ToLower(e.Name())] = e.Name()
	}

	for _, w := range expected {
		t.Run(w.name, func(t *testing.T) {
			onDisk, ok := available[strings.ToLower(w.name)]
			if !ok {
				t.Skipf("not present in this install: %s", w.name)
			}
			path := filepath.Join(dir, onDisk)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			f, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if len(f.Records) != w.count {
				t.Errorf("records = %d, want %d", len(f.Records), w.count)
			}
			if got := len(f.Records) * RecordSize; got != len(data) {
				t.Errorf("byte total %d, want %d", got, len(data))
			}
		})
	}
}
