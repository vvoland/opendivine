// SPDX-License-Identifier: GPL-3.0-only

package world

import (
	"os"
	"path/filepath"
	"testing"

	"grono.dev/opendivine/internal/testutils"
)

// TestRealPartition walks the shipped main/startup/world.x0 from
// $TEST_GAMEDATA_PATH.
func TestRealPartition(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	path := filepath.Join(gamedata, "main/startup/world.x0")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	cells, objects := 0, 0
	withFloor := 0
	withOverlay := 0
	tileIDs := map[int16]int{}
	if err := Walk(data, func(x, y int, c Cell) {
		cells++
		objects += int(c.ObjectCount)
		if c.FloorTileID != 0 {
			withFloor++
		}
		if c.OverlayTile != -1 {
			withOverlay++
		}
		tileIDs[c.FloorTileID]++
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	t.Logf("cells=%d, objects=%d, with floor=%d, with overlay=%d, distinct tile ids=%d",
		cells, objects, withFloor, withOverlay, len(tileIDs))
	if cells == 0 {
		t.Fatal("no cells walked")
	}
}
