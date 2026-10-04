// SPDX-License-Identifier: GPL-3.0-only

// Package collision implements the engine's movement collision
// (re_docs/formats/collide.md): there is no per-move geometric test —
// each placed object's collide cube is RASTERIZED into a cell-flag
// grid at placement (an inclusive rectangle in iso cell space), and
// movers test whole cells by flag mask. Doors change state by
// re-rasterizing with a new mask. The grid also carries the map's terrain
// heights ([Grid.LoadHeights]) and the engine's walkability test
// ([Grid.WalkableUV]).
package collision

import "grono.dev/opendivine/pkg/assets/height"

// Cell-flag bits, as stamped by the engine's rasterizer and tested by
// its walkability function (fcn.0056f3c0). Movers test with
// MaskStatic|MaskPlayerBlock|MaskAgent (0x13).
const (
	MaskStatic      uint16 = 0x0001 // hard blocker (default objects; locked doors; map records)
	MaskPlayerBlock uint16 = 0x0002 // sb_player_block objects
	MaskDoorClosed  uint16 = 0x0004 // closed door (sight/line tests)
	MaskSight       uint16 = 0x0008 // sb_no_look_through
	MaskAgent       uint16 = 0x0010 // an agent's leg-destination occupancy
	MaskLight       uint16 = 0x0080 // sb_light
	MaskLever       uint16 = 0x0100 // sb_lever
	MaskSaved       uint16 = 0x0200 // the map's own blocker (see [Grid.LoadHeights])
	MaskWalkOn      uint16 = 0x0400 // sb_walk_on climb cells (height-gated)
)

// MoverMask is what the engine's movement steppers test candidate
// cells with (mask 0x13 at the fcn.0056f3c0 call sites).
const MoverMask = MaskStatic | MaskPlayerBlock | MaskAgent

// gridMax is the engine's valid-index bound (index <= 0x200800 gates
// every grid access).
const gridMax = 0x200800

// ClimbLimit is the engine's step-up limit, `cmp …,0x50` — 80 height
// units. A walk-on cell rising more than this above the cell being
// stepped out of is rejected outright, not costed (re_docs/pathfinding.md).
const ClimbLimit = 0x50

// Step is one entry of the engine's 16-direction isometric step table
// (DAT_00654e50, `{i32 du, i32 dv} × 16`) in cell space — the table the
// walkability test, the steppers and the A* core all index.
type Step struct{ Du, Dv int }

// Dir16 is that table, direction 0 = N rotating clockwise
// (re_docs/formats/collide.md). The steps are isometric rather than square
// 8-way, so the 8 odd entries are the half-step intermediates that cover
// more than one cell per leg.
var Dir16 = [16]Step{
	0:  {Du: -1, Dv: -1}, // N
	1:  {Du: -3, Dv: -2},
	2:  {Du: -2, Dv: -1},
	3:  {Du: -3, Dv: -1},
	4:  {Du: -1, Dv: 0}, // W
	5:  {Du: -1, Dv: 1},
	6:  {Du: 0, Dv: 1},
	7:  {Du: 1, Dv: 2},
	8:  {Du: 1, Dv: 1}, // S
	9:  {Du: 3, Dv: 2},
	10: {Du: 2, Dv: 1},
	11: {Du: 3, Dv: 1},
	12: {Du: 1, Dv: 0}, // E
	13: {Du: 1, Dv: -1},
	14: {Du: 0, Dv: -1},
	15: {Du: -1, Dv: -2},
}

// corners[i] are the extra cell offsets (Δu, Δv) that direction i's leg
// passes through and that must be clear too, on top of its destination: the
// engine's 16-case switch at 0x5719a8 inside fcn.0056f3c0
// (re_docs/formats/collide.md). Directions 4/6/12/13/14 are "dest only".
var corners = [16][][2]int{
	0:  {{-1, -1}, {-1, 0}},
	1:  {{-2, -2}, {-2, -1}, {-1, -1}, {-1, 0}, {-2, 0}, {0, -1}},
	2:  {{-1, -1}, {-1, 0}},
	3:  {{-2, -1}, {-1, -1}, {-1, 0}},
	4:  nil,
	5:  {{-1, 0}, {0, 1}},
	6:  nil,
	7:  {{1, 1}, {0, 1}, {1, 0}},
	8:  {{0, 1}, {1, 0}},
	9:  {{2, 2}, {2, 1}, {2, 0}, {1, 0}, {1, 1}},
	10: {{1, 1}, {0, 1}, {1, 0}},
	11: {{2, 1}, {1, 1}, {1, 0}, {2, 0}, {3, 0}, {0, 1}},
	12: nil,
	13: nil,
	14: nil,
	15: {{-1, -1}, {0, -1}},
}

// Grid is the walkability grid: the engine's worldmap cell array
// ([0x74eca0]) reduced to the fields movement needs. Cells are
// addressed by the engine's own index formula v<<10 + u with
// u = (x+y)>>5, v = y>>5 — including its aliasing for u > 1023, so
// stamps and tests always agree with the original.
type Grid struct {
	flags []uint16
	// blockRef is the engine's per-cell blocker refcount (the top
	// nibble of the flags word in the original; kept separate here):
	// stamps with MaskStatic|MaskPlayerBlock increment it, removals
	// decrement, and flag bits clear only when it reaches zero.
	blockRef []uint8
	// climb and base are the cell's height bytes, cell+6 and cell+7 of the
	// engine's 8-byte cell record (re_docs/cell-grid.md): the walk-on object
	// add-on in quarter-units and the terrain height.
	climb []uint8
	base  []uint8
}

// NewGrid returns an empty walkability grid.
func NewGrid() *Grid {
	return &Grid{
		flags:    make([]uint16, gridMax+1),
		blockRef: make([]uint8, gridMax+1),
		climb:    make([]uint8, gridMax+1),
		base:     make([]uint8, gridMax+1),
	}
}

// cellIndex maps world coordinates to the engine's cell index:
// u = (x+y)>>5, v = y>>5, index = v<<10 + u (fcn.0056d720 and every
// other site). ok is false outside the engine's index gate.
func cellIndex(x, y int) (int, bool) {
	u := (x + y) >> 5
	v := y >> 5
	idx := v<<10 + u
	return idx, idx >= 0 && idx <= gridMax
}

// CellIndex maps world (x, y) to the engine's cell index (v<<10 + u,
// u = (x+y)>>5, v = y>>5); ok is false outside the engine's index gate.
// Exposed so callers (e.g. a debug overlay) can tell which single cell
// a mover occupies — the engine's movers carry no radius and own
// exactly one cell.
func CellIndex(x, y int) (idx int, ok bool) {
	return cellIndex(x, y)
}

// CellUV returns the (u, v) cell coordinates for world (x, y):
// u = (x+y)>>5, v = y>>5. Movers work in (u, v) space to apply the
// engine's per-direction corner-cell offsets.
func CellUV(x, y int) (u, v int) {
	return (x + y) >> 5, y >> 5
}

// BlockedUV reports whether cell (u, v) has any of the mask bits set.
// Out-of-range cells (past the engine's index gate) count as blocked.
func (g *Grid) BlockedUV(u, v int, mask uint16) bool {
	idx := v<<10 + u
	if idx < 0 || idx > gridMax {
		return true
	}
	return g.flags[idx]&mask != 0
}

// Blocked reports whether the cell containing world (x, y) has any of
// the mask bits set — the plain single-cell test. A move is validated
// with [Grid.WalkableUV], which adds the direction's corner chain and
// the climb gate.
func (g *Grid) Blocked(x, y int, mask uint16) bool {
	idx, ok := cellIndex(x, y)
	if !ok {
		return true // out of the engine's grid = not walkable
	}
	return g.flags[idx]&mask != 0
}

// cellInBounds is fcn.0056f3c0's entry gate on the cell the test is
// called with: coordinates at least 4, and the iso extents (u−v)*32 <
// 0x7f00 and v*32 < 0xff00, which are the world-pixel extents
// (re_docs/pathfinding.md). The A* core bounds-checks its own nodes.
func cellInBounds(u, v int) bool {
	return u >= 4 && v >= 4 && (u-v)*32 < 0x7f00 && v*32 < 0xff00
}

// LoadHeights writes the grid's terrain layer from a decoded height.x<n> —
// the engine's per-cell loader (FUN_0056d260), the only thing that fills the
// base byte and the only blocker source that is not an object
// (re_docs/cell-grid.md):
//
//	cell[7] = height
//	if (cell[7] != 0)  flags |= 0x400
//	if (saved & 0x200) { flags |= 0x201; flags &= ~0x400; refcount++ }
//
// So raised terrain is climb-gated by construction (the file's own 0x400 is
// a stale editor bit), and a map-blocked cell outranks that with a refcount
// of its own.
//
// It must run before the objects are stamped: [Grid.Stamp] measures a walk-on
// cube's climb byte down from the base height and puts back the climb bit this
// clears on a blocker cell (TestLoadHeightsBeforeStamps). Both sides index by
// v<<10 + u, so cell i of the file is cell i of the grid. It returns what it
// wrote — cells blocked, and cells left climb-gated — which the stamps that
// follow would blur.
func (g *Grid) LoadHeights(m *height.Map) (blocked, climb int) {
	for i, h := range m.Heights {
		g.base[i] = h
		if h != 0 {
			g.flags[i] |= MaskWalkOn
		}
		if m.Blocker(i) {
			g.flags[i] |= MaskStatic | MaskSaved
			g.flags[i] &^= MaskWalkOn
			if g.blockRef[i] < 255 {
				g.blockRef[i]++
			}
			blocked++
		} else if h != 0 {
			climb++
		}
	}
	return blocked, climb
}

// SetHeightsUV sets cell (u, v)'s climb and base height bytes — how a test
// supplies the map's relief directly, without a height.x<n>.
func (g *Grid) SetHeightsUV(u, v int, climb, base uint8) {
	idx := v<<10 + u
	if idx < 0 || idx > gridMax {
		return
	}
	g.climb[idx], g.base[idx] = climb, base
}

// HeightsUV returns cell (u, v)'s climb and base height bytes, both zero
// outside the grid.
func (g *Grid) HeightsUV(u, v int) (climb, base uint8) {
	idx := v<<10 + u
	if idx < 0 || idx > gridMax {
		return 0, 0
	}
	return g.climb[idx], g.base[idx]
}

// WalkableUV reports whether a leg in direction dir out of cell (u, v) is
// allowed — fcn.0056f3c0, the one gate movement shares with the A* core
// and the reach tests. It checks the bounds gate, the destination cell
// Dir16[dir] and every corner cell of the chain: each is rejected on the
// mask blocker bits and, on a walk-on cell, on the climb gate.
func (g *Grid) WalkableUV(u, v, dir int, mask uint16) bool {
	if !cellInBounds(u, v) {
		return false
	}
	d := Dir16[dir]
	if !g.stepInto(u, v, u+d.Du, v+d.Dv, mask) {
		return false
	}
	for _, c := range corners[dir] {
		if !g.stepInto(u, v, u+c[0], v+c[1], mask) {
			return false
		}
	}
	return true
}

// stepInto tests one examined cell of a leg leaving (u, v): the blocker bits,
// then — on a walk-on cell — the climb gate:
//
//	delta = (height0 + height1) − current_height;  delta > 0x50: reject
//
// The neighbour contributes both of its height bytes and the cell being left
// only its climb byte (re_docs/formats/collide.md, re_docs/pathfinding.md).
func (g *Grid) stepInto(u, v, nu, nv int, mask uint16) bool {
	n := nv<<10 + nu
	if n < 0 || n > gridMax {
		return false // out of the engine's grid = not walkable
	}
	f := g.flags[n]
	if f&mask != 0 {
		return false
	}
	if f&MaskWalkOn == 0 {
		return true
	}
	var cur uint8
	if idx := v<<10 + u; idx >= 0 && idx <= gridMax {
		cur = g.climb[idx]
	}
	return int(g.climb[n])+int(g.base[n])-int(cur) <= ClimbLimit
}

// Cube is one object's stamp parameters, kept so the object can
// re-rasterize on state change (the engine's remove+add on door
// toggle).
type Cube struct {
	X, Y    int // object world position + collide record anchor_x/anchor_y
	XExtent int // collide i16[3]: spans the iso u axis
	Width   int // collide i16[5]: width/2 spans the v axis

	// ZHeight is the collide record's z_height (i16[4]), the z extent of the
	// cube's slab; BaseZ is the object's own ground Z, added to it. Only
	// walk-on cubes read them, to work out how high the step onto them is.
	ZHeight int
	BaseZ   int

	Mask uint16
}

// cells calls fn for every cell of the cube's inclusive rectangle,
// exactly per the engine rasterizer fcn.0056d720:
//
//	u ∈ [(x+y)>>5, (x+y+x_extent)>>5],  v ∈ [(y−width/2)>>5, y>>5]
func (c *Cube) cells(fn func(idx int)) {
	u1 := (c.X + c.Y) >> 5
	u2 := (c.X + c.Y + c.XExtent) >> 5
	v1 := (c.Y - c.Width/2) >> 5
	v2 := c.Y >> 5
	for v := v1; v <= v2; v++ {
		for u := u1; u <= u2; u++ {
			idx := v<<10 + u
			if idx >= 0 && idx <= gridMax {
				fn(idx)
			}
		}
	}
}

// ContainsCell reports whether world (x, y) falls in one of the cells
// of the cube's stamp rectangle.
func (c *Cube) ContainsCell(x, y int) bool {
	pu := (x + y) >> 5
	pv := y >> 5
	u1 := (c.X + c.Y) >> 5
	u2 := (c.X + c.Y + c.XExtent) >> 5
	v1 := (c.Y - c.Width/2) >> 5
	v2 := c.Y >> 5
	return pu >= u1 && pu <= u2 && pv >= v1 && pv <= v2
}

// Stamp rasterizes the cube into the grid (fcn.0056d720): OR the mask
// into each cell, counting hard blockers. A zero mask stamps nothing.
func (g *Grid) Stamp(c Cube) {
	if c.Mask == 0 {
		return
	}
	c.cells(func(idx int) {
		g.flags[idx] |= c.Mask
		if c.Mask&(MaskStatic|MaskPlayerBlock) != 0 && g.blockRef[idx] < 255 {
			g.blockRef[idx]++
		}
		// A walk-on cube records how high the step onto it is, in
		// quarter-units, keeping the tallest contributor (fcn.0056d720):
		// byte[+6] = (h − 4·byte[+7] + 3) >> 2, with h = z_height + baseZ.
		// The byte store wraps for a cube over 1020 units tall, as the
		// original's does.
		if c.Mask&MaskWalkOn != 0 {
			if d := c.ZHeight + c.BaseZ - 4*int(g.base[idx]); d > 4*int(g.climb[idx]) {
				g.climb[idx] = uint8((d + 3) >> 2)
			}
		}
	})
}

// Unstamp is the inverse (fcn.0056d890): decrement the blocker
// refcount and clear the cube's bits only when no other blocker still
// owns the cell.
func (g *Grid) Unstamp(c Cube) {
	if c.Mask == 0 {
		return
	}
	c.cells(func(idx int) {
		if c.Mask&(MaskStatic|MaskPlayerBlock) != 0 && g.blockRef[idx] > 0 {
			g.blockRef[idx]--
		}
		if g.blockRef[idx] == 0 {
			g.flags[idx] &^= c.Mask
		}
	})
}

// ObjectState is the flags-word state the mask derivation reads
// (instance s_* bits + two catalogue sb_* behaviour bits).
type ObjectState struct {
	PlayerBlock   bool // s_player_block
	WalkThrough   bool // s_walk_through
	Door          bool // s_door
	Closed        bool // s_closed (runtime state)
	Locked        bool // s_locked (runtime state)
	Light         bool // s_light
	Lever         bool // s_lever
	WalkOn        bool // catalogue sb_walk_on
	NoLookThrough bool // catalogue sb_no_look_through
}

// ObjectMask derives the cell mask from the object flags word, in the
// engine's exact order (fcn.00572100 / fcn.0056e2c0; note the cube
// `type` field plays no part):
//
//	if sb_player_block: mask = walk_through ? 0 : 0x2
//	elif sb_door:       mask = (closed ? 0x4 : 0) | (locked ? 0x1 : 0)
//	else:               mask = walk_through ? 0 : 0x1
//	plus light|lever|walk_on|no_look_through extras;
//	hard blockers strip the climb bit.
func ObjectMask(o ObjectState) uint16 {
	var mask uint16
	switch {
	case o.PlayerBlock:
		if !o.WalkThrough {
			mask = MaskPlayerBlock
		}
	case o.Door:
		if o.Closed {
			mask |= MaskDoorClosed
		}
		if o.Locked {
			mask |= MaskStatic
		}
	default:
		if !o.WalkThrough {
			mask = MaskStatic
		}
	}
	if o.Light {
		mask |= MaskLight
	}
	if o.Lever {
		mask |= MaskLever
	}
	if o.WalkOn {
		mask |= MaskWalkOn
	}
	if o.NoLookThrough {
		mask |= MaskSight
	}
	if mask&MaskStatic != 0 {
		mask &^= MaskWalkOn
	}
	return mask
}
