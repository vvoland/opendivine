// SPDX-License-Identifier: GPL-3.0-only

// Package mover implements the engine's leg-based movement stepper
// (re_docs/formats/collide.md): an agent does not slide freely through
// world space and re-derive its cell every frame. It commits one
// discrete "leg" at a time — pick one of 16 iso directions from the
// current cell whose destination cell improves the octagonal distance
// to the goal AND passes walkability, mark that destination cell as the
// occupancy, then interpolate the float position toward it over
// Walkcount frames with no mid-leg collision recheck (0x40f0a0 stepper →
// 0x427630/0x4270a0 commit → 0x427d30 per-frame advance).
//
// Because the occupancy cell is first-class discrete state advanced one
// cell per leg (never a floor() of a continuously moving position), it
// tracks the sprite in lockstep and has none of the lead/lag asymmetry
// that a position→cell mapping shows.
package mover

import (
	"math"

	"grono.dev/opendivine/internal/game/collision"
)

// vel is one of the 16 iso directions' motion parameters — the velocity
// table 0x654f50 (re_docs/formats/collide.md), indexed by the same
// direction as the cell-step table [collision.Dir16] (0x654e50).
type vel struct {
	Vx, Vy float64 // per-frame world velocity in cells (32 px = 1.0), ×speed
	Factor int     // Walkcount multiplier: 1 primary facing, 2 half-step
}

// vel16 lists the directions 0 = N rotating clockwise. The per-leg world
// delta is (32·(Du−Dv), 32·Dv) from [collision.Dir16], and the float
// velocity is that delta over Walkcount (checked in the test).
var vel16 = [16]vel{
	0:  {Vx: 0.0, Vy: -1.0, Factor: 1}, // N
	1:  {Vx: -0.5, Vy: -1.0, Factor: 2},
	2:  {Vx: -1.0, Vy: -1.0, Factor: 1},
	3:  {Vx: -1.0, Vy: -0.5, Factor: 2},
	4:  {Vx: -1.0, Vy: 0.0, Factor: 1}, // W
	5:  {Vx: -1.0, Vy: 0.5, Factor: 2},
	6:  {Vx: -1.0, Vy: 1.0, Factor: 1},
	7:  {Vx: -0.5, Vy: 1.0, Factor: 2},
	8:  {Vx: 0.0, Vy: 1.0, Factor: 1}, // S
	9:  {Vx: 0.5, Vy: 1.0, Factor: 2},
	10: {Vx: 1.0, Vy: 1.0, Factor: 1},
	11: {Vx: 1.0, Vy: 0.5, Factor: 2},
	12: {Vx: 1.0, Vy: 0.0, Factor: 1}, // E
	13: {Vx: 1.0, Vy: -0.5, Factor: 2},
	14: {Vx: 1.0, Vy: -1.0, Factor: 1},
	15: {Vx: 0.5, Vy: -1.0, Factor: 2},
}

// cellPx is the iso cell edge in world pixels (the >>5 in the cell index).
const cellPx = 32

// dirAngle[i] is direction i's world heading (atan2 of its velocity),
// used to pick the direction that best faces the goal.
var dirAngle = func() [16]float64 {
	var a [16]float64
	for i := range vel16 {
		a[i] = math.Atan2(vel16[i].Vy, vel16[i].Vx)
	}
	return a
}()

// angDiff returns the absolute angular difference in [0, π].
func angDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 2*math.Pi)
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return d
}

// OctDistance is the engine's octagonal distance (fcn.0040ecb0), used as
// the stepper's greedy score and the A* heuristic:
//
//	dist = 1.0390625·max(|dx|,|dy|) + 0.3984375·min(|dx|,|dy|)
func OctDistance(dx, dy float64) float64 {
	dx, dy = math.Abs(dx), math.Abs(dy)
	hi, lo := dx, dy
	if lo > hi {
		hi, lo = lo, hi
	}
	return 1.0390625*hi + 0.3984375*lo
}

// Mover holds an agent's smooth float position and its current leg. The
// occupancy cell is the current leg's destination (or the standing cell
// when idle).
type Mover struct {
	X, Y  float64 // float world position (engine Fx/Fy)
	Speed float64 // px/frame (hero walk = 2)

	grid *collision.Grid
	mask uint16

	// Current leg. remain == 0 means idle (no leg in progress).
	dir    int
	remain int
	vx, vy float64 // per-frame velocity for the active leg

	// occX, occY is the leg destination in world px — the point the leg
	// interpolates toward and whose cell is the occupancy cell.
	occX, occY float64
}

// New returns a mover standing at (x, y).
func New(grid *collision.Grid, mask uint16, x, y, speed float64) *Mover {
	return &Mover{X: x, Y: y, Speed: speed, grid: grid, mask: mask, occX: x, occY: y}
}

// Moving reports whether a leg is in progress.
func (m *Mover) Moving() bool { return m.remain > 0 }

// CellIndex returns the occupancy cell (leg destination, or standing cell
// when idle) — what a debug overlay should highlight as the agent's cell.
func (m *Mover) CellIndex() (int, bool) {
	return collision.CellIndex(int(m.occX), int(m.occY))
}

// Update advances the mover one frame toward (goalX, goalY) when active,
// and returns the world delta applied this frame (0,0 when idle) so the
// caller can drive facing/animation.
//
// Unlike the engine's AI agents — which commit a whole leg and never
// re-check until it ends — the player mover stays responsive: it re-aims
// the moment the best direction changes and stops immediately when
// inactive (keyboard control would feel sluggish otherwise). Speed, the
// 16-direction set, walkability and the per-cell leg length are still the
// engine's; only the multi-frame *commitment* is relaxed. Because the
// occupancy cell is still the chosen direction's destination (never a
// floor() of the live position), it tracks the sprite symmetrically with
// no lead/lag.
func (m *Mover) Update(goalX, goalY float64, active bool) (dx, dy float64) {
	if !active {
		m.stop()
		return 0, 0
	}
	best := m.bestDir(goalX, goalY)
	if best < 0 {
		m.stop() // arrived, or walled in on every improving heading
		return 0, 0
	}
	// (Re)commit when idle or when the goal now wants a different heading.
	if m.remain == 0 || best != m.dir {
		m.commit(best)
	}
	m.X += m.vx
	m.Y += m.vy
	m.remain--
	if m.remain == 0 {
		// Completed a full leg cleanly: land on the cell to kill float drift.
		m.X, m.Y = m.occX, m.occY
	}
	return m.vx, m.vy
}

// stop clears any leg and parks the occupancy on the standing cell.
func (m *Mover) stop() {
	m.remain = 0
	m.vx, m.vy = 0, 0
	m.occX, m.occY = m.X, m.Y
}

// bestDir returns the direction to step toward the goal, or -1 when none
// both reduces the octagonal distance and is walkable. It prefers the
// heading that best faces the goal, then fans out to steadily larger
// deviations — so a clear path goes straight and a blocked one slides
// (the blocked best-heading falls through to the next-best walkable one).
func (m *Mover) bestDir(goalX, goalY float64) int {
	dgx, dgy := goalX-m.X, goalY-m.Y
	curDist := OctDistance(dgx, dgy)
	want := math.Atan2(dgy, dgx)

	order := [16]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	sortByDeviation(&order, want)

	u0, v0 := collision.CellUV(int(m.X), int(m.Y))
	for _, i := range order {
		d := collision.Dir16[i]
		destX := m.X + float64(cellPx*(d.Du-d.Dv))
		destY := m.Y + float64(cellPx*d.Dv)
		if OctDistance(goalX-destX, goalY-destY) >= curDist {
			continue // this step does not get us closer
		}
		if !m.grid.WalkableUV(u0, v0, i, m.mask) {
			continue
		}
		return i
	}
	return -1
}

// commit starts a leg in direction i: the engine's per-cell frame count
// and velocity, with the occupancy cell set to the destination.
func (m *Mover) commit(i int) {
	d := collision.Dir16[i]
	v := &vel16[i]
	m.dir = i
	m.remain = v.Factor * int(math.Round(cellPx/m.Speed))
	m.vx = v.Vx * m.Speed
	m.vy = v.Vy * m.Speed
	m.occX = m.X + float64(cellPx*(d.Du-d.Dv))
	m.occY = m.Y + float64(cellPx*d.Dv)
}

// sortByDeviation orders the 16 direction indices by increasing angular
// distance of their heading from want (insertion sort; the slice is
// tiny and this keeps the ordering stable for ties).
func sortByDeviation(order *[16]int, want float64) {
	for i := 1; i < len(order); i++ {
		j := i
		for j > 0 && angDiff(dirAngle[order[j]], want) < angDiff(dirAngle[order[j-1]], want) {
			order[j], order[j-1] = order[j-1], order[j]
			j--
		}
	}
}
