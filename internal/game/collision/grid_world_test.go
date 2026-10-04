// SPDX-License-Identifier: GPL-3.0-only

package collision_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"grono.dev/opendivine/internal/game/collision"
	"grono.dev/opendivine/internal/testutils"
	"grono.dev/opendivine/pkg/assets/collide"
	"grono.dev/opendivine/pkg/assets/height"
	"grono.dev/opendivine/pkg/assets/location"
	"grono.dev/opendivine/pkg/assets/objects"
	"grono.dev/opendivine/pkg/assets/world"
)

// shippedHeights decodes a shipped height.x<n>.
func shippedHeights(t *testing.T, n int) *height.Map {
	t.Helper()
	name := filepath.Join(testutils.TestGameData(t), "main/startup/static",
		fmt.Sprintf("height.x%d", n))
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	m, err := height.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return m
}

// TestShippedWorldWalkable builds the walkability grid from the real
// world.x0 the same way the game does and sanity-checks it: the hero
// spawn cell must be free and most of its neighbourhood walkable —
// guarding against mask or anchor regressions that would wall the
// player in (cell blocking is coarse, so a systematic offset error
// shows up here immediately).
func TestShippedWorldWalkable(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	mustRead := func(rel string) []byte {
		b, err := os.ReadFile(filepath.Join(gamedata, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return b
	}
	cat, err := objects.Decode(bytes.NewReader(mustRead("static/objects.000")))
	if err != nil {
		t.Fatal(err)
	}
	col, err := collide.Decode(bytes.NewReader(mustRead("static/imagelists/Collide.0")))
	if err != nil {
		t.Fatal(err)
	}
	locs, err := location.Decode(bytes.NewReader(mustRead("global/location.000")))
	if err != nil {
		t.Fatal(err)
	}

	grid := collision.NewGrid()
	// The terrain layer first, exactly as loadRegion does it: the map's
	// own blockers are part of what the player walks into.
	grid.LoadHeights(shippedHeights(t, 0))
	// typeZeroWalls counts hard-blocker cubes whose collide record has
	// type 0, and remembers one cube's anchor to assert it blocks. These
	// walls (e.g. the tall stone walls in the region-0 start room, mask
	// 0x009) were passable under the old `cr.Type != 0` placement gate;
	// the engine ignores cube type and gates on the flags-word mask
	// (fcn.00572100 @ 0x5721ec).
	typeZeroWalls := 0
	var wallX, wallY int
	err = world.Walk(mustRead("main/startup/world.x0"), func(cx, cy int, c world.Cell) {
		for _, o := range c.Objects {
			id := int(o.CatalogueID)
			if id >= len(col.Records) || id >= len(cat.Entries) {
				continue
			}
			cr := col.Records[id]
			e := cat.Entries[id]
			door := e.HasS(objects.SDoor)
			// The engine gates the rasterize call on the flags-word mask,
			// not the cube type (fcn.00572100 @ 0x5721ec); mask 0 stamps
			// nothing.
			mask := collision.ObjectMask(collision.ObjectState{
				PlayerBlock:   e.HasS(objects.SPlayerBlock),
				WalkThrough:   e.HasS(objects.SWalkThrough),
				Door:          door,
				Closed:        door && e.HasS(objects.SClosed),
				Locked:        e.HasS(objects.SLocked),
				Light:         e.HasS(objects.SLight),
				Lever:         e.HasS(objects.SLever),
				WalkOn:        e.HasSB(objects.SBWalkOn),
				NoLookThrough: e.HasSB(objects.SBNoLookThrough),
			})
			if mask == 0 {
				continue
			}
			x := cx + int(o.SubX) + int(cr.AnchorX)
			y := cy + int(o.SubY) + int(cr.AnchorY)
			if cr.Type == 0 && mask&collision.MaskStatic != 0 {
				typeZeroWalls++
				wallX, wallY = x, y
			}
			grid.Stamp(collision.Cube{
				X:       x,
				Y:       y,
				XExtent: int(cr.XExtent),
				Width:   int(cr.Width),
				Mask:    mask,
			})
			stamped++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stamped %d cubes (%d of them type-0 hard blockers)", stamped, typeZeroWalls)

	// Regression: type-0 walls must block. The old gate skipped every
	// cube with cr.Type == 0, so the player walked through these walls.
	if typeZeroWalls == 0 {
		t.Fatal("expected some type-0 hard-blocker walls in world.x0")
	}
	if !grid.Blocked(wallX, wallY, collision.MaskStatic) {
		t.Errorf("type-0 wall anchor (%d,%d) is not blocked — the type gate regressed", wallX, wallY)
	}

	var sx, sy int
	for _, r := range locs.Records {
		if r.Name == "stps_hero" {
			sx, sy = int(r.V0), int(r.V1)
		}
	}
	if sx == 0 && sy == 0 {
		t.Fatal("no stps_hero spawn record")
	}
	mask := collision.MoverMask | collision.MaskDoorClosed
	if grid.Blocked(sx, sy, mask) {
		t.Errorf("hero spawn cell (%d,%d) is blocked", sx, sy)
	}
	free := 0
	for dy := -160; dy <= 160; dy += 32 {
		for dx := -160; dx <= 160; dx += 32 {
			if !grid.Blocked(sx+dx, sy+dy, mask) {
				free++
			}
		}
	}
	t.Logf("free cells around spawn: %d/121", free)
	if free < 60 {
		t.Errorf("only %d/121 cells around the spawn are walkable — the grid looks over-blocked", free)
	}
}

// TestShippedHeightMapLoads pins the census of what the real height.x0 puts
// into the walkability grid: the climb bit comes from the height byte, so
// 140,170 of region 0's cells are climb-gated — not the 132,411 that carry
// the file's own stale 0x400 — and the 78,547 with the saved 0x200 are
// hard-blocked by the map itself.
func TestShippedHeightMapLoads(t *testing.T) {
	m := shippedHeights(t, 0)
	grid := collision.NewGrid()
	blocked, climb := grid.LoadHeights(m)
	if blocked != 78547 || climb != 140170 {
		t.Errorf("height.x0 census = %d blocked, %d climb-gated; want 78547, 140170", blocked, climb)
	}

	// Two sampled cells, one of each kind: the first cell with a height
	// (index 400628 = u 244, v 391, height 20) and the first the map
	// blocks (index 16427 = u 43, v 16, saved word 0x1201).
	if c, b := grid.HeightsUV(244, 391); c != 0 || b != 20 {
		t.Errorf("cell (244,391) heights = (%d,%d), want (0,20)", c, b)
	}
	if !grid.BlockedUV(244, 391, collision.MaskWalkOn) {
		t.Error("cell (244,391) has a height but no climb bit")
	}
	if grid.BlockedUV(244, 391, collision.MoverMask) {
		t.Error("cell (244,391) is a plain raised cell, it must not block movement")
	}
	if !grid.BlockedUV(43, 16, collision.MaskStatic|collision.MaskSaved) {
		t.Error("cell (43,16) carries the saved 0x200 bit but does not block")
	}
}

// heightFile builds a synthetic height.x<n> image with the named cells
// filled in: one 3-byte record per grid cell, `{u8 height; u16 flags}`.
func heightFile(cells map[int][2]uint16) []byte {
	b := make([]byte, height.FileBytes)
	for i, rec := range cells {
		b[i*height.RecordBytes] = uint8(rec[0])
		binary.LittleEndian.PutUint16(b[i*height.RecordBytes+1:], rec[1])
	}
	return b
}

// TestLoadedMapBlocker pins the other half of the loader: a saved 0x200
// blocks movement outright, keeps no climb bit, and holds a refcount of its
// own — so unstamping an object that shared the cell leaves it standing.
func TestLoadedMapBlocker(t *testing.T) {
	const u, v = 300, 150
	m, err := height.Decode(bytes.NewReader(heightFile(map[int][2]uint16{
		height.Index(u, v): {40, uint16(height.FlagBlocker)},
	})))
	if err != nil {
		t.Fatal(err)
	}
	g := collision.NewGrid()
	blocked, climb := g.LoadHeights(m)
	if blocked != 1 || climb != 0 {
		t.Errorf("census = %d blocked, %d climb; want 1, 0", blocked, climb)
	}
	if !g.BlockedUV(u, v, collision.MoverMask) {
		t.Fatal("the map's blocker does not block movement")
	}
	if g.BlockedUV(u, v, collision.MaskWalkOn) {
		t.Error("a map-blocked cell kept the climb bit the height byte derived")
	}
	// A barrel on the same cell, then removed.
	barrel := collision.Cube{X: (u - v) * 32, Y: v * 32, Mask: collision.MaskStatic}
	g.Stamp(barrel)
	g.Unstamp(barrel)
	if !g.BlockedUV(u, v, collision.MoverMask) {
		t.Error("removing an object from a map-blocked cell unblocked it — the loader's refcount is missing")
	}
}

// TestLoadHeightsBeforeStamps is the order [collision.Grid.LoadHeights]
// documents and `loadRegion` obeys. Both of the loader's writes are
// order-sensitive: [collision.Grid.Stamp] measures a walk-on cube's climb
// byte down from the cell's base height, and the loader clears the walk-on
// bit on a cell the map blocks, which a stamp before it loses.
func TestLoadHeightsBeforeStamps(t *testing.T) {
	const u, v = 400, 200
	const terrain, crate = 20, 320
	// A raised cell to step onto, and a map-blocked one beside it.
	m, err := height.Decode(bytes.NewReader(heightFile(map[int][2]uint16{
		height.Index(u+1, v): {terrain, 0},
		height.Index(u+2, v): {terrain, uint16(height.FlagBlocker)},
	})))
	if err != nil {
		t.Fatal(err)
	}
	cube := func(cu int) collision.Cube {
		return collision.Cube{X: (cu - v) * 32, Y: v * 32, ZHeight: crate, Mask: collision.MaskWalkOn}
	}
	build := func(heightsFirst bool) *collision.Grid {
		g := collision.NewGrid()
		if heightsFirst {
			g.LoadHeights(m)
		}
		g.Stamp(cube(u + 1))
		g.Stamp(cube(u + 2))
		if !heightsFirst {
			g.LoadHeights(m)
		}
		return g
	}

	right := build(true)
	if c, b := right.HeightsUV(u+1, v); c != 60 || b != terrain {
		t.Errorf("loader first: heights = (%d,%d), want (60,%d) — (320−4·20+3)>>2", c, b, terrain)
	}
	// 60 + 20 = exactly ClimbLimit, so the step is still allowed.
	if !right.WalkableUV(u, v, 12, collision.MoverMask) {
		t.Error("loader first: the 320-unit crate on terrain 20 is not a step, want walkable")
	}
	if !right.BlockedUV(u+2, v, collision.MaskWalkOn) {
		t.Error("loader first: the stamp did not put the walk-on bit back on the map-blocked cell")
	}

	wrong := build(false)
	if c, _ := wrong.HeightsUV(u+1, v); c != 80 {
		t.Fatalf("stamping first: climb byte = %d, want 80 — the fixture no longer separates the two orders", c)
	}
	if wrong.WalkableUV(u, v, 12, collision.MoverMask) {
		t.Error("stamping first: the same crate is still a step — the climb byte is not measured from the base height")
	}
	if wrong.BlockedUV(u+2, v, collision.MaskWalkOn) {
		t.Error("stamping first: the map-blocked cell kept a walk-on bit the loader clears")
	}
}
