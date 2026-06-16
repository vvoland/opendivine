// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"grono.dev/opendivine/internal/game/character"
	"grono.dev/opendivine/pkg/assets/objects"
)

// newInteractGame builds a Game with a single interactive object at (1000,1000)
// whose collider (index 0) starts enabled, plus the player at (px,py).
func newInteractGame(px, py float64) *Game {
	g := &Game{
		player: &character.Character{X: px, Y: py},
		colliders: []collider{
			{box: aabb{X: 994, Y: 994, W: 12, H: 12}, enabled: true},
		},
		insts: []objectInst{
			{
				X: 1000, Y: 1000, ObjID: 1, Elev: 50, SpriteW: 40, SpriteH: 100,
				Interactive: true, ToggleCollider: true, ColliderIdx: 0,
			},
		},
	}
	return g
}

func TestTryInteract(t *testing.T) {
	tests := []struct {
		name        string
		px, py      float64 // player position
		cx, cy      float64 // click position
		wantHandled bool
		wantOpen    bool // door state afterward
	}{
		{
			name: "click near door in reach opens it",
			px:   1000, py: 1040, cx: 1020, cy: 1000,
			wantHandled: true, wantOpen: true,
		},
		{
			name: "click visible sprite above foot opens it",
			px:   1000, py: 1040, cx: 1020, cy: 960,
			wantHandled: true, wantOpen: true,
		},
		{
			name: "click on door but player too far walks instead",
			px:   1000, py: 1400, cx: 1020, cy: 1000,
			wantHandled: false, wantOpen: false,
		},
		{
			name: "click near foot but outside sprite is not handled",
			px:   1000, py: 1010, cx: 990, cy: 1000,
			wantHandled: false, wantOpen: false,
		},
		{
			name: "click empty space is not handled",
			px:   1000, py: 1010, cx: 1300, cy: 1300,
			wantHandled: false, wantOpen: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newInteractGame(tt.px, tt.py)
			got := g.tryInteract(tt.cx, tt.cy)
			if got != tt.wantHandled {
				t.Errorf("tryInteract handled = %v, want %v", got, tt.wantHandled)
			}
			if g.insts[0].Open != tt.wantOpen {
				t.Errorf("door Open = %v, want %v", g.insts[0].Open, tt.wantOpen)
			}
			// The collider is enabled exactly when the door is closed.
			if enabled := g.colliders[0].enabled; enabled == g.insts[0].Open {
				t.Errorf("collider enabled = %v with Open = %v; want opposite",
					enabled, g.insts[0].Open)
			}
		})
	}
}


func TestTryInteractUsesColliderForReach(t *testing.T) {
	g := &Game{
		player: &character.Character{X: 1000, Y: 1040},
		colliders: []collider{
			{box: aabb{X: 994, Y: 1014, W: 12, H: 12}, enabled: true},
		},
		insts: []objectInst{
			{
				X: 1000, Y: 900, ObjID: 1, SpriteW: 20, SpriteH: 140,
				Interactive: true, ToggleCollider: true, ColliderIdx: 0,
			},
		},
	}

	if !g.tryInteract(1010, 950) {
		t.Fatalf("tryInteract handled = false, want true")
	}
	if !g.insts[0].Open {
		t.Errorf("ladder-like object Open = false, want true")
	}
}

func TestTryInteractConsumesUseClassObjectWithoutToggling(t *testing.T) {
	g := &Game{
		player: &character.Character{X: 1000, Y: 1040},
		insts: []objectInst{
			{
				X: 1000, Y: 900, ObjID: 1, SpriteW: 20, SpriteH: 140,
				Interactive: true,
			},
		},
	}

	handled := g.tryInteract(1010, 950)

	assert.Check(t, cmp.Equal(handled, true))
	assert.Check(t, cmp.Equal(g.insts[0].Open, false))
}

func TestObjectInteractionFlagsDoesNotToggleWalls(t *testing.T) {
	wall := &objects.Object{SBFlags: objects.SBLightBlocker}

	interactive, toggleCollider := objectInteractionFlags(wall, 2)

	assert.Check(t, cmp.Equal(interactive, false))
	assert.Check(t, cmp.Equal(toggleCollider, false))
}

// Opening then closing a door restores its blocker, and a door cannot be closed
// while the player stands in its cell.
func TestUseObjectToggle(t *testing.T) {
	g := newInteractGame(1000, 1040) // in reach, not on the collider
	in := &g.insts[0]

	g.useObject(in) // open
	if !in.Open || g.colliders[0].enabled {
		t.Fatalf("after open: Open=%v enabled=%v, want true/false", in.Open, g.colliders[0].enabled)
	}

	g.useObject(in) // close again
	if in.Open || !g.colliders[0].enabled {
		t.Fatalf("after close: Open=%v enabled=%v, want false/true", in.Open, g.colliders[0].enabled)
	}

	// Open it, then stand the player on the collider: it must not close.
	g.useObject(in) // open
	g.player.X, g.player.Y = 1000, 1000
	g.useObject(in) // attempt close while standing in the doorway
	if !in.Open {
		t.Errorf("door closed onto the player; want it to stay open")
	}
	if g.colliders[0].enabled {
		t.Errorf("collider re-enabled under the player; want it to stay passable")
	}
}

// playerBlocked must ignore a disabled (open-door) collider.
func TestPlayerBlockedRespectsEnabled(t *testing.T) {
	g := newInteractGame(1000, 1000)
	g.colliderGrid = map[int][]int32{}
	// Bucket the single collider into its 64px cell, mirroring loadRegion.
	cx, cy := 994/cellPx, 994/cellPx
	g.colliderGrid[cy*worldCellsX+cx] = []int32{0}

	if !g.playerBlocked(1000, 1000) {
		t.Fatalf("closed door should block the player")
	}
	g.colliders[0].enabled = false
	if g.playerBlocked(1000, 1000) {
		t.Errorf("open door should not block the player")
	}
}
