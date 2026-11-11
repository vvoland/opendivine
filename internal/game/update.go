// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"grono.dev/opendivine/internal/game/collision"
	"grono.dev/opendivine/pkg/assets/objects"
)

var regionKeys = []ebiten.Key{
	ebiten.Key0, ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4,
}

func (g *Game) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	g.advanceObjectAnimations()
	for n, k := range regionKeys {
		if ebiten.IsKeyPressed(k) && g.region != n {
			if err := g.loadRegion(n); err != nil {
				log.Printf("region %d: %v", n, err)
			}
			break
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF7) {
		g.showFloors = !g.showFloors
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF8) {
		g.showObjects = !g.showObjects
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
		g.showColliders = !g.showColliders
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		g.showRegions = !g.showRegions
		if g.showRegions && !g.namedRegionsLoaded {
			g.loadNamedRegions()
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF12) {
		g.wantShot = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF9) {
		g.cameraFollow = !g.cameraFollow
		if g.cameraFollow {
			g.camX, g.camY = g.player.CameraTarget()
		}
	}
	// [ / ] cycle through anim slots.  \ disables the override
	// and returns to auto walk/idle.
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
		s := g.player.ForceSlot
		if s < 0 {
			s = g.player.AnimSlot
		}
		s--
		if s < 0 {
			s = 18
		}
		g.player.ForceSlot = s
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
		s := g.player.ForceSlot
		if s < 0 {
			s = g.player.AnimSlot
		}
		s++
		if s > 18 {
			s = 0
		}
		g.player.ForceSlot = s
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackslash) {
		g.player.ForceSlot = -1
	}
	// Movement speed in world pixels per tick. The old loop advanced at 60 TPS
	// with a 2 px/tick player speed; use 3 px/tick at 40 TPS to preserve its
	// 120 px/s pace while keeping the simulation at the engine's rate.
	// Shift is a debug fast-walk with no engine counterpart.
	speed := heroWalkSpeed
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		speed *= 4
	}

	// Left-click sets a click-to-walk destination at the world point
	// under the cursor.  The hero then walks toward it, leg by leg, until
	// arrival or a wall it cannot get around.  WASD overrides and cancels
	// the destination.
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		// Inverse of worldToScreen.
		wx := (float64(mx)-float64(g.winW)/2.0)/g.zoom + g.camX
		wy := (float64(my)-float64(g.winH)/2.0)/g.zoom + g.camY
		// A click on an in-reach interactive object uses it; otherwise it's a
		// click-to-walk target.
		if !g.tryInteract(wx, wy) {
			g.destX, g.destY = wx, wy
			g.hasDest = true
		}
	}

	// Raw key heading (−1/0/+1 per axis); the stepper turns it into legs.
	hx, hy := 0.0, 0.0
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		hy -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		hy += 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		hx -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		hx += 1
	}
	wasdActive := hx != 0 || hy != 0
	if wasdActive {
		g.hasDest = false // any keyboard input cancels the click target
	}

	if g.cameraFollow {
		// Leg-based stepper (re_docs/formats/collide.md, internal/game/mover):
		// choose a goal, advance one leg-frame, then sync the sprite to the
		// mover. The mover owns a discrete occupancy cell advanced one cell
		// per leg, so there is no free-slide / floor()-of-position jitter.
		g.mover.Speed = speed
		var goalX, goalY float64
		active := false
		switch {
		case wasdActive:
			// Aim far along the held heading; each leg re-aims to the nearest
			// walkable 16-direction, which is what makes it slide along walls.
			goalX = g.mover.X + hx*4096
			goalY = g.mover.Y + hy*4096
			active = true
		case g.hasDest:
			goalX, goalY = g.destX, g.destY
			active = true
		}
		vx, vy := g.mover.Update(goalX, goalY, active)
		g.player.X = g.mover.X
		g.player.Y = g.mover.Y
		g.player.Step(vx, vy)
		// Click target reached or fully walled: stop retrying it.
		if g.hasDest && !wasdActive && vx == 0 && vy == 0 && !g.mover.Moving() {
			g.hasDest = false
		}
		g.camX, g.camY = g.player.CameraTarget()
	} else {
		// Free pan, slower at higher zoom for fine framing.
		g.camX = clamp(g.camX+hx*speed/g.zoom, 0, worldXPx)
		g.camY = clamp(g.camY+hy*speed/g.zoom, 0, worldYPx)
	}
	if _, scrollY := ebiten.Wheel(); scrollY != 0 {
		g.zoom *= 1.0 + 0.1*scrollY
		if g.zoom < 1.0/64.0 {
			g.zoom = 1.0 / 64.0
		}
		if g.zoom > 4.0 {
			g.zoom = 4.0
		}
	}
	return nil
}

// useReach is how close (world px) the player's foot must be to an interactive
// object to use it — roughly 1.5 cells.
const useReach = 96.0

// tryInteract uses the interactive object nearest the click point, if the
// player is within reach of it. Returns true when it handled the click so the
// caller skips click-to-walk.
//
// This is the world-visible core of the engine's CObject::Use door/chest path
// (re_docs/object-interaction.md): flip sb_closed and un/re-occupy the
// collision grid. Open-frame animation, sounds, locks/keys, levers and
// scripted (Osiris) objects are intentionally not wired yet.
func (g *Game) tryInteract(wx, wy float64) bool {
	// Pick the topmost interactive object whose rendered sprite contains the
	// click. This matches what the player sees; falling back to the old foot
	// radius only covers objects whose sprite dimensions could not be loaded.
	best := g.objectAtWorld(wx, wy, true)
	if best < 0 {
		return false
	}
	in := &g.insts[best]
	// Too far to reach: let the click fall through to walk toward it; the
	// player can click again once adjacent. (Auto-use-on-arrival is TBD.)
	if g.interactionDistance(in) > useReach {
		return false
	}
	g.useObject(in)
	return true
}

func (g *Game) interactionDistance(in *objectInst) float64 {
	if in.ColliderIdx >= 0 && in.ColliderIdx < len(g.colliders) {
		return pointAABBDistance(g.player.X, g.player.Y, g.colliders[in.ColliderIdx].box)
	}

	w, h := in.SpriteW, in.SpriteH
	if (w <= 0 || h <= 0) && g.objReader != nil {
		if e, err := g.objReader.Entry(in.ObjID); err == nil {
			w = int(e.Width)
			h = int(e.Height)
		}
	}
	if w > 0 && h > 0 {
		return pointAABBDistance(g.player.X, g.player.Y, aabb{
			X: in.X,
			Y: in.Y - in.Elev,
			W: w,
			H: h,
		})
	}

	return math.Hypot(float64(in.X)-g.player.X, float64(in.Y)-g.player.Y)
}

func pointAABBDistance(px, py float64, box aabb) float64 {
	minX := float64(box.X)
	maxX := float64(box.X + box.W)
	minY := float64(box.Y)
	maxY := float64(box.Y + box.H)

	dx := 0.0
	if px < minX {
		dx = minX - px
	} else if px > maxX {
		dx = px - maxX
	}

	dy := 0.0
	if py < minY {
		dy = minY - py
	} else if py > maxY {
		dy = py - maxY
	}

	return math.Hypot(dx, dy)
}

func (g *Game) objectAtWorld(wx, wy float64, interactiveOnly bool) int {
	best := -1
	for i := range g.insts {
		in := &g.insts[i]
		if interactiveOnly && !in.Interactive {
			continue
		}
		if !g.objectContainsWorld(in, wx, wy) {
			continue
		}
		best = i
	}
	return best
}

func (g *Game) objectContainsWorld(in *objectInst, wx, wy float64) bool {
	w, h := in.SpriteW, in.SpriteH
	if (w <= 0 || h <= 0) && g.objReader != nil {
		if e, err := g.objReader.Entry(in.ObjID); err == nil {
			w = int(e.Width)
			h = int(e.Height)
		}
	}
	if w > 0 && h > 0 {
		x := float64(in.X)
		y := float64(in.Y - in.Elev)
		return wx >= x && wx < x+float64(w) && wy >= y && wy < y+float64(h)
	}

	const footHitR2 = 28.0 * 28.0
	dx := float64(in.X) - wx
	dy := float64(in.Y) - wy
	return dx*dx+dy*dy < footHitR2
}

// useObject toggles a door/chest between open and closed. State
// changes re-rasterize the object's cube with a freshly derived mask —
// the engine's remove+add on CObject::Use (re_docs/formats/collide.md)
// — so an open door stamps nothing and a closed one blocks again.
// sb_locked gates opening outright (key-based unlocking is not wired
// yet). A door is never closed onto the player.
func (g *Game) useObject(in *objectInst) {
	if in.Locked && !in.Open {
		return
	}
	if in.Open && in.ToggleCollider && in.ColliderIdx >= 0 && g.playerOnCollider(in.ColliderIdx) {
		return
	}
	in.Open = !in.Open
	if in.ToggleCollider && in.ColliderIdx >= 0 {
		c := &g.colliders[in.ColliderIdx]
		g.walkGrid.Unstamp(c.cube)
		var cat *objects.Object
		if g.catalog != nil && in.ObjID >= 0 && in.ObjID < len(g.catalog.Entries) {
			cat = &g.catalog.Entries[in.ObjID]
		}
		c.cube.Mask = objectMask(cat, in)
		g.walkGrid.Stamp(c.cube)
	}
}

// playerOnCollider reports whether the player stands on a cell of the
// collider's stamp rectangle, so a door can't be closed onto the
// player (the engine's movers are cell-granular, so cell membership is
// the natural test).
func (g *Game) playerOnCollider(idx int) bool {
	return g.colliders[idx].cube.ContainsCell(int(g.player.X), int(g.player.Y))
}

// playerMask is what player movement tests candidate cells with. The
// engine's behaviour steppers use mask 0x13 (static | player-block |
// agent occupancy); we additionally include the closed-door bit so the
// player cannot walk through a closed unlocked door — the original
// gameplay clearly blocks there, but the exact mask the player path
// uses is not pinned in collide.md yet (the stepper mask is).
const playerMask = collision.MoverMask | collision.MaskDoorClosed

// heroWalkSpeed is the hero's base walk speed in world pixels per tick.
const heroWalkSpeed = 4.0

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
