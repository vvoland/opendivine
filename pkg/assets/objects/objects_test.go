// SPDX-License-Identifier: GPL-3.0-only

package objects

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"grono.dev/opendivine/internal/testutils"
)

// TestRealCatalog parses the shipped static/objects.000 from
// $TEST_GAMEDATA_PATH and spot-checks known entries.
func TestRealCatalog(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	path := filepath.Join(gamedata, "static/objects.000")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	cat, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := len(cat.Entries); got != 7208 {
		t.Errorf("entry count = %d, want 7208 (parallel to CPacked imagelist 0)", got)
	}

	// Spot check known entries (verified against CSV exporter output).
	for _, want := range []struct {
		idx  int
		name string
	}{
		{0, "Dead bush"},
		{100, "Rock wall"},
		{156, "Tree"},
		{274, "Metal Shield"},
		{1000, "Rocks"},
	} {
		got := cat.Entries[want.idx]
		if got.ID != uint32(want.idx) {
			t.Errorf("entry %d: ID = %d, want %d", want.idx, got.ID, want.idx)
		}
		if got.Name != want.name {
			t.Errorf("entry %d: Name = %q, want %q", want.idx, got.Name, want.name)
		}
	}

	// Count entries with sb_force_floor set — these are the floor objects.
	floorCount := 0
	for _, o := range cat.Entries {
		if o.HasSB(SBForceFloor) {
			floorCount++
		}
	}
	t.Logf("entries with sb_force_floor: %d", floorCount)
}
