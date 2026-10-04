// SPDX-License-Identifier: GPL-3.0-only

package mover

import (
	"math"
	"testing"

	"grono.dev/opendivine/internal/game/collision"
)

// TestDirTableConsistent locks the velocity table (0x654f50) against the
// cell-step table (0x654e50, collision.Dir16): per-leg world delta is
// (32·(Du−Dv), 32·Dv) and the per-frame velocity is that delta over
// Walkcount, i.e. ((Du−Dv)/Factor, Dv/Factor).
func TestDirTableConsistent(t *testing.T) {
	for i, v := range vel16 {
		d := collision.Dir16[i]
		wantVx := float64(d.Du-d.Dv) / float64(v.Factor)
		wantVy := float64(d.Dv) / float64(v.Factor)
		if v.Vx != wantVx || v.Vy != wantVy {
			t.Errorf("dir %d: Vx,Vy = (%.2f,%.2f), want (%.2f,%.2f) from Du,Dv=%d,%d factor=%d",
				i, v.Vx, v.Vy, wantVx, wantVy, d.Du, d.Dv, v.Factor)
		}
	}
}

func TestOctDistance(t *testing.T) {
	cases := []struct {
		dx, dy, want float64
	}{
		{10, 0, 10.390625},
		{0, -10, 10.390625},
		{10, 10, 10.390625 + 3.984375},
	}
	for _, c := range cases {
		if got := OctDistance(c.dx, c.dy); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("OctDistance(%.0f,%.0f) = %v, want %v", c.dx, c.dy, got, c.want)
		}
	}
}

// runToRest steps the mover toward the goal until it stops (or a frame
// cap), returning the frame count.
func runToRest(m *Mover, goalX, goalY float64) int {
	for f := range 100000 {
		m.Update(goalX, goalY, true)
		if !m.Moving() {
			// Try to start another leg; if still idle it has arrived/blocked.
			m.Update(goalX, goalY, true)
			if !m.Moving() {
				return f
			}
		}
	}
	return -1
}

// TestStraightEastLeg checks a single primary leg east: exactly one cell
// (32 px) over 16 frames at speed 2, landing precisely, with the
// occupancy cell one step east of the start.
func TestStraightEastLeg(t *testing.T) {
	g := collision.NewGrid()
	m := New(g, collision.MoverMask, 1000, 1000, 2)
	startCell, _ := collision.CellIndex(1000, 1000)

	for f := range 16 {
		if !m.Moving() && f > 0 {
			t.Fatalf("leg ended early at frame %d", f)
		}
		m.Update(2000, 1000, true)
	}
	if m.Moving() {
		t.Errorf("primary leg should finish in 16 frames at speed 2")
	}
	if m.X != 1032 || m.Y != 1000 {
		t.Errorf("after one east leg: pos = (%.1f,%.1f), want (1032,1000)", m.X, m.Y)
	}
	occ, _ := m.CellIndex()
	eastCell, _ := collision.CellIndex(1032, 1000)
	if occ != eastCell || occ == startCell {
		t.Errorf("occupancy cell did not advance exactly one step east")
	}
}

// TestReachesGoal walks a diagonal goal on an open grid and lands within
// one cell of it.
func TestReachesGoal(t *testing.T) {
	g := collision.NewGrid()
	m := New(g, collision.MoverMask, 1000, 1000, 2)
	goalX, goalY := 1600.0, 1480.0
	if frames := runToRest(m, goalX, goalY); frames < 0 {
		t.Fatal("did not arrive within the frame cap")
	}
	if d := OctDistance(goalX-m.X, goalY-m.Y); d > cellPx {
		t.Errorf("stopped %.1f from goal (> one cell); pos=(%.1f,%.1f)", d, m.X, m.Y)
	}
}

// TestSlideAlongBlocker blocks the cell directly east of the start and
// checks the mover still makes eastward progress (it re-aims to a
// walkable diagonal) without ever entering the blocked cell.
func TestSlideAlongBlocker(t *testing.T) {
	g := collision.NewGrid()
	// Block a band of cells due east at the start row so the straight
	// path is walled but diagonals around it are open.
	blockedX := 1000 + cellPx
	g.Stamp(collision.Cube{X: blockedX, Y: 1000, XExtent: 0, Width: 0, Mask: collision.MaskStatic})
	blockedCell, _ := collision.CellIndex(blockedX, 1000)

	m := New(g, collision.MoverMask, 1000, 1000, 2)
	startX := m.X
	for f := range 200 {
		m.Update(4000, 1000, true)
		if occ, ok := m.CellIndex(); ok && occ == blockedCell {
			t.Fatalf("mover entered the blocked cell at frame %d", f)
		}
	}
	if m.X <= startX {
		t.Errorf("mover made no eastward progress around the blocker: %.1f -> %.1f", startX, m.X)
	}
}

// TestStopMidLeg checks the player mover halts the instant it goes
// inactive — mid-leg, without gliding to the cell boundary (keyboard
// release must feel immediate).
func TestStopMidLeg(t *testing.T) {
	g := collision.NewGrid()
	m := New(g, collision.MoverMask, 1000, 1000, 2)
	for range 5 { // partway into an east leg
		m.Update(2000, 1000, true)
	}
	if !m.Moving() {
		t.Fatal("expected to be mid-leg after 5 frames")
	}
	x := m.X
	dx, dy := m.Update(2000, 1000, false)
	if dx != 0 || dy != 0 || m.Moving() {
		t.Errorf("did not stop on release: d=(%.1f,%.1f) moving=%v", dx, dy, m.Moving())
	}
	if m.X != x {
		t.Errorf("glided after release: %.1f -> %.1f", x, m.X)
	}
}

// TestReaimMidLeg checks a change of goal re-aims within one frame rather
// than finishing the committed direction first.
func TestReaimMidLeg(t *testing.T) {
	g := collision.NewGrid()
	m := New(g, collision.MoverMask, 1000, 1000, 2)
	for range 4 { // heading east
		m.Update(2000, 1000, true)
	}
	// Now aim due south; the very next frame should carry southward motion.
	_, dy := m.Update(1000, 2000, true)
	if dy <= 0 {
		t.Errorf("did not re-aim toward the new goal within a frame: dy=%.1f", dy)
	}
}

// TestIdleWhenNotActive does not move without an active goal.
func TestIdleWhenNotActive(t *testing.T) {
	g := collision.NewGrid()
	m := New(g, collision.MoverMask, 1000, 1000, 2)
	dx, dy := m.Update(2000, 1000, false)
	if dx != 0 || dy != 0 || m.Moving() {
		t.Errorf("mover moved while inactive: d=(%.1f,%.1f) moving=%v", dx, dy, m.Moving())
	}
}
