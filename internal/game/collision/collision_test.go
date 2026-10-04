// SPDX-License-Identifier: GPL-3.0-only

package collision

import "testing"

// TestObjectMask locks down the engine's mask derivation order
// (re_docs/formats/collide.md): player_block wins over door, doors
// contribute closed/locked bits (an open unlocked door stamps
// nothing), walk_through kills default blockers, and hard blockers
// strip the climb bit.
func TestObjectMask(t *testing.T) {
	tests := []struct {
		name string
		o    ObjectState
		want uint16
	}{
		{"default object blocks", ObjectState{}, MaskStatic},
		{"walk-through decal stamps nothing", ObjectState{WalkThrough: true}, 0},
		{"closed unlocked door", ObjectState{Door: true, Closed: true}, MaskDoorClosed},
		{"closed locked door", ObjectState{Door: true, Closed: true, Locked: true}, MaskDoorClosed | MaskStatic},
		{"open unlocked door stamps nothing", ObjectState{Door: true}, 0},
		{"player-block", ObjectState{PlayerBlock: true}, MaskPlayerBlock},
		{"player-block walk-through", ObjectState{PlayerBlock: true, WalkThrough: true}, 0},
		{"player-block wins over door", ObjectState{PlayerBlock: true, Door: true, Closed: true}, MaskPlayerBlock},
		{"walk-on stair", ObjectState{WalkThrough: true, WalkOn: true}, MaskWalkOn},
		{"hard blocker strips climb", ObjectState{WalkOn: true}, MaskStatic},
		{"light and lever extras", ObjectState{Light: true, Lever: true}, MaskStatic | MaskLight | MaskLever},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ObjectMask(tc.o); got != tc.want {
				t.Errorf("ObjectMask(%+v) = %#x, want %#x", tc.o, got, tc.want)
			}
		})
	}
}

// TestStampRectangle pins the rasterizer's inclusive cell rectangle
// (fcn.0056d720): u spans (x+y)>>5 .. (x+y+x_extent)>>5 and v spans
// (y−width/2)>>5 .. y>>5.
func TestStampRectangle(t *testing.T) {
	g := NewGrid()
	// x+y = 1024 → u ∈ [32, 35] (x_extent 96); y = 512 → v ∈ [14, 16]
	// (width 128 → y−64).
	c := Cube{X: 512, Y: 512, XExtent: 96, Width: 128, Mask: MaskStatic}
	g.Stamp(c)

	for _, p := range []struct{ x, y int }{
		{512, 512},           // anchor cell (u32, v16)
		{512 + 96, 512},      // right edge (u35, v16)
		{512 + 64, 512 - 64}, // top edge (v14), u still in range
	} {
		if !g.Blocked(p.x, p.y, MoverMask) {
			t.Errorf("cell at (%d,%d) not blocked, want blocked", p.x, p.y)
		}
	}
	// Just outside on each axis.
	if g.Blocked(512+96+32, 512, MoverMask) { // u36
		t.Error("cell right of the rect blocked, want free")
	}
	if g.Blocked(512, 512-96, MoverMask) { // v13 (u drops to 29 — also outside)
		t.Error("cell above the rect blocked, want free")
	}
	if g.Blocked(512-33, 512, MoverMask) { // u31
		t.Error("cell left of the rect blocked, want free")
	}
}

// TestRefcountOverlap pins the blocker refcount semantics
// (fcn.0056d890): where two blockers overlap, removing one leaves the
// cell blocked; removing both frees it.
func TestRefcountOverlap(t *testing.T) {
	g := NewGrid()
	a := Cube{X: 512, Y: 512, Width: 64, Mask: MaskStatic}
	b := Cube{X: 512, Y: 512, Width: 64, Mask: MaskStatic}
	g.Stamp(a)
	g.Stamp(b)
	g.Unstamp(a)
	if !g.Blocked(512, 512, MoverMask) {
		t.Error("overlapped cell freed by removing one of two blockers")
	}
	g.Unstamp(b)
	if g.Blocked(512, 512, MoverMask) {
		t.Error("cell still blocked after removing both blockers")
	}
}

// TestDoorToggle pins the remove+add door transition: a closed locked
// door blocks movers; re-stamping it open and unlocked frees the cell
// while an unrelated wall in the same cell keeps blocking.
func TestDoorToggle(t *testing.T) {
	g := NewGrid()
	door := Cube{X: 512, Y: 512, Width: 64,
		Mask: ObjectMask(ObjectState{Door: true, Closed: true, Locked: true})}
	g.Stamp(door)
	if !g.Blocked(512, 512, MoverMask) {
		t.Fatal("locked door does not block")
	}
	g.Unstamp(door)
	door.Mask = ObjectMask(ObjectState{Door: true}) // open, unlocked
	g.Stamp(door)
	if g.Blocked(512, 512, MoverMask) {
		t.Error("open door still blocks movers")
	}

	wall := Cube{X: 512, Y: 512, Width: 64, Mask: MaskStatic}
	g.Stamp(wall)
	if !g.Blocked(512, 512, MoverMask) {
		t.Error("wall sharing the door's cell does not block")
	}
}

// TestOutOfGrid: coordinates outside the engine's index gate are not
// walkable.
func TestOutOfGrid(t *testing.T) {
	g := NewGrid()
	if !g.Blocked(-100, -100, MoverMask) {
		t.Error("negative-index cell walkable, want blocked")
	}
}

// stampCellUV stamps a single-cell cube carrying mask at cell (u, v):
// x+y = u·32 and y = v·32 puts the rasterizer's rectangle on exactly it.
func stampCellUV(g *Grid, u, v int, mask uint16) {
	g.Stamp(Cube{X: (u - v) * 32, Y: v * 32, Mask: mask})
}

// TestClimbGate pins the 80-unit climb limit: exactly at it is still a
// step, one unit more is a cliff.
//
// The last two cases pin which byte `current_height` is. The gate
// subtracts the climb byte of the cell being left alone, so its terrain
// byte counts for nothing no matter how high it stands; subtracting both
// would make the raised-terrain case walkable (see [Grid.stepInto] for
// what the reading costs on the shipped maps).
func TestClimbGate(t *testing.T) {
	const u, v = 200, 100
	tests := []struct {
		name              string
		curClimb, curBase uint8
		nClimb, nBase     uint8
		wantWalkable      bool
	}{
		{name: "level walk-on cell", wantWalkable: true},
		{name: "step of exactly the limit", nClimb: 80, wantWalkable: true},
		{name: "one unit over the limit", nClimb: 81},
		{name: "limit split across both height bytes", nClimb: 40, nBase: 41},
		{name: "step down off a tall cell", curClimb: 200, nClimb: 10, wantWalkable: true},
		// Standing on terrain twice as high as the cell ahead does not
		// help: the base byte of the cell being left is not in the sum.
		{name: "raised terrain under the cell being left counts for nothing", curBase: 200, nBase: 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGrid()
			// Direction 12 is the pure +u step and is "dest only", so
			// nothing but the destination cell can decide this test.
			stampCellUV(g, u+1, v, MaskWalkOn)
			g.SetHeightsUV(u, v, tc.curClimb, tc.curBase)
			g.SetHeightsUV(u+1, v, tc.nClimb, tc.nBase)
			if got := g.WalkableUV(u, v, 12, MoverMask); got != tc.wantWalkable {
				t.Errorf("WalkableUV over a walk-on cell (climb %d, base %d; from climb %d, base %d) = %v, want %v",
					tc.nClimb, tc.nBase, tc.curClimb, tc.curBase, got, tc.wantWalkable)
			}
		})
	}
}

// TestWalkableCornerChain checks the corner/pass-through chain: a blocker
// in a cell the leg only clips past still cancels it, so a diagonal cannot
// cut a wall corner. Direction 8 examines Δ(0,1) and Δ(1,0) besides its
// destination; direction 12 examines nothing but its destination.
func TestWalkableCornerChain(t *testing.T) {
	const u, v = 200, 100
	g := NewGrid()
	stampCellUV(g, u+1, v, MaskStatic) // a corner cell of dir 8, dest of dir 12
	if g.WalkableUV(u, v, 8, MoverMask) {
		t.Error("dir 8 walkable with its Δ(1,0) corner cell blocked")
	}
	if g.WalkableUV(u, v, 12, MoverMask) {
		t.Error("dir 12 walkable into a blocked destination cell")
	}
	if !g.WalkableUV(u, v, 6, MoverMask) { // (0,1), dest only, clear
		t.Error("dir 6 not walkable although its destination is clear")
	}
}

// TestStampClimbHeight pins the rasterizer's climb-height write: a walk-on
// cube records (h − 4·base + 3) >> 2 in quarter-units, keeping the tallest
// contributor.
func TestStampClimbHeight(t *testing.T) {
	g := NewGrid()
	const u, v = 300, 120
	x, y := (u-v)*32, v*32
	g.Stamp(Cube{X: x, Y: y, ZHeight: 148, Mask: MaskWalkOn})
	if climb, _ := g.HeightsUV(u, v); climb != 37 { // (148+3)>>2
		t.Errorf("climb byte = %d, want 37", climb)
	}
	g.Stamp(Cube{X: x, Y: y, ZHeight: 8, Mask: MaskWalkOn}) // shorter, ignored
	if climb, _ := g.HeightsUV(u, v); climb != 37 {
		t.Errorf("a shorter walk-on cube lowered the climb byte to %d, want 37", climb)
	}
	g.Stamp(Cube{X: x, Y: y, ZHeight: 200, BaseZ: 200, Mask: MaskWalkOn})
	if climb, _ := g.HeightsUV(u, v); climb != 100 { // (200+200+3)>>2
		t.Errorf("climb byte = %d, want 100", climb)
	}
	// A cube with no walk-on bit never touches the height bytes.
	g2 := NewGrid()
	g2.Stamp(Cube{X: x, Y: y, ZHeight: 148, Mask: MaskStatic})
	if climb, _ := g2.HeightsUV(u, v); climb != 0 {
		t.Errorf("non-walk-on cube wrote climb byte %d, want 0", climb)
	}
}

// TestWalkableBoundsGate covers fcn.0056f3c0's entry gate.
func TestWalkableBoundsGate(t *testing.T) {
	g := NewGrid()
	if g.WalkableUV(3, 100, 12, MoverMask) {
		t.Error("cell u=3 walkable, want the bounds gate to reject it")
	}
	if g.WalkableUV(100, 3, 12, MoverMask) {
		t.Error("cell v=3 walkable, want the bounds gate to reject it")
	}
	if !g.WalkableUV(4, 4, 12, MoverMask) {
		t.Error("cell (4,4) not walkable, want the gate to admit it")
	}
}
