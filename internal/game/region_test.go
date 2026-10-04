// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"grono.dev/opendivine/internal/game/character"
	"grono.dev/opendivine/internal/game/collision"
	"grono.dev/opendivine/internal/testutils"
	"grono.dev/opendivine/pkg/assets/collide"
	"grono.dev/opendivine/pkg/assets/height"
	"grono.dev/opendivine/pkg/assets/objects"
)

// regionGame is a Game with just enough of New() to make loadRegion do the
// whole job: without the catalogue and Collide.0 the object pass stamps
// nothing, and a grid with no objects in it cannot show how the terrain
// layer and the object stamps combine.
func regionGame(t *testing.T, dir string) *Game {
	t.Helper()
	g := &Game{gameDir: dir, player: &character.Character{}}
	cf, err := os.Open(filepath.Join(dir, "static/objects.000"))
	assert.NilError(t, err)
	defer cf.Close()
	g.catalog, err = objects.Decode(cf)
	assert.NilError(t, err)
	colf, err := os.Open(filepath.Join(dir, "static/imagelists/Collide.0"))
	assert.NilError(t, err)
	defer colf.Close()
	g.collide0, err = collide.Decode(colf)
	assert.NilError(t, err)
	return g
}

// TestLoadRegionLoadsTheHeightMap is the wiring assertion: a region load
// puts static/height.x<n> into the walkability grid the game moves on. Every
// cell's base height is compared against the file, and the map's own
// blockers — the 0x200 bit no object mask carries — are counted.
//
// The last count is the call-site ORDER: a walk-on object on a cell the map
// blocks gets back the climb bit the loader had just cleared, so it is 0 if
// loadHeights runs after the object pass.
func TestLoadRegionLoadsTheHeightMap(t *testing.T) {
	dir := testutils.TestGameData(t)
	g := regionGame(t, dir)
	assert.NilError(t, g.loadRegion(0))
	assert.Assert(t, len(g.colliders) > 0, "the object pass stamped nothing — the fixture is not the game")

	f, err := os.Open(filepath.Join(dir, "main/startup/static/height.x0"))
	assert.NilError(t, err)
	defer f.Close()
	m, err := height.Decode(f)
	assert.NilError(t, err)

	wrongBase, mapBlockers, reMarked := 0, 0, 0
	for i := range height.Cells {
		u, v := i%height.Cols, i/height.Cols
		if _, base := g.walkGrid.HeightsUV(u, v); base != m.Height(i) {
			wrongBase++
		}
		if !g.walkGrid.BlockedUV(u, v, collision.MaskSaved) {
			continue
		}
		mapBlockers++
		if g.walkGrid.BlockedUV(u, v, collision.MaskWalkOn) {
			reMarked++
		}
	}
	assert.Equal(t, wrongBase, 0, "cells whose base height is not the file's")
	assert.Equal(t, mapBlockers, 78547, "cells blocked by height.x0 itself")
	assert.Assert(t, reMarked > 0,
		"no map-blocked cell kept a walk-on bit: loadHeights ran AFTER the object pass")
}

// TestLoadRegionRequiresTheHeightMap pins the one place in loadRegion where a
// missing file is FATAL: the catalogue and Collide.0 fall back, but
// height.x<n> is not an overlay — 78,547 of region 0's walls come out of it
// and nothing else supplies them.
//
// The fixture holds nothing but world.x0, so the load reaches loadHeights.
func TestLoadRegionRequiresTheHeightMap(t *testing.T) {
	real := testutils.TestGameData(t)
	tests := []struct {
		name  string
		bytes int64 // < 0: write no file at all
		want  string
	}{
		{"no file at all", -1, "open height.x0"},
		{"one byte short of the grid", int64(height.FileBytes) - 1, "read grid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			assert.NilError(t, os.MkdirAll(filepath.Join(dir, "main/startup/static"), 0o755))
			assert.NilError(t, os.Symlink(
				filepath.Join(real, "main/startup/world.x0"),
				filepath.Join(dir, "main/startup/world.x0")))
			if tc.bytes >= 0 {
				// Sparse: the content does not matter, only the length.
				f, err := os.Create(filepath.Join(dir, "main/startup/static/height.x0"))
				assert.NilError(t, err)
				assert.NilError(t, f.Truncate(tc.bytes))
				assert.NilError(t, f.Close())
			}

			g := &Game{gameDir: dir, player: &character.Character{}}
			err := g.loadRegion(0)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
