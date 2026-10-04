// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"grono.dev/opendivine/internal/game/collision"
	"grono.dev/opendivine/internal/game/mover"
	"grono.dev/opendivine/pkg/assets/height"
	"grono.dev/opendivine/pkg/assets/objects"
	"grono.dev/opendivine/pkg/assets/world"
)

// Tile 274 is true void (pure black); everything else is a real floor
// texture (tile 0 is dirt, NOT void).
const tileVoid = 274

// loadRegion reloads world.x<n> into g.cells and g.insts, clearing tile/sprite
// caches.
func (g *Game) loadRegion(n int) error {
	worldPath := fmt.Sprintf("%s/main/startup/world.x%d", g.gameDir, n)
	worldBytes, err := os.ReadFile(worldPath)
	if err != nil {
		return fmt.Errorf("read world.x%d: %w", n, err)
	}
	g.region = n
	g.cells = g.cells[:0]
	g.insts = g.insts[:0]
	g.colliders = g.colliders[:0]
	g.walkGrid = collision.NewGrid()
	g.floorTiles = map[int16]*ebiten.Image{}
	g.objSprites = map[int]*sprite{}

	if err := g.loadHeights(n); err != nil {
		return err
	}

	if err := world.Walk(worldBytes, func(cellX, cellY int, c world.Cell) {
		hasFloor := c.FloorTileID != tileVoid

		// Cells with floor=0 AND no overlay can also still be skipped, the engine
		// renders the screen background through them.
		// But cells with floor != 0 must always be drawn.
		hasOverlay := c.OverlayTile >= 0
		if hasFloor || hasOverlay {
			g.cells = append(g.cells, floorCell{
				CellX:     uint16(cellX),
				CellY:     uint16(cellY),
				FloorID:   c.FloorTileID,
				OverlayID: c.OverlayTile,
			})
		}
		for _, o := range c.Objects {
			// Layer ALWAYS contributes to elevation, including for walls.
			// An earlier hack zeroed elevation for SBLightBlocker but that
			// broke door lintels (e.g. id=45 layer=112: a thin top-piece
			// sitting above a doorway, would render at the bottom of the door
			// without the layer applied).
			catID := int(o.CatalogueID)
			wx := cellX + int(o.SubX)
			wy := cellY + int(o.SubY)
			inst := objectInst{
				X:              wx,
				Y:              wy,
				ObjID:          catID,
				Layer:          int(o.Layer),
				AnimationIndex: -1,
				Elev:           int(o.Layer),
				ColliderIdx:    -1,
			}
			var cat *objects.Object
			if g.catalog != nil && catID >= 0 && catID < len(g.catalog.Entries) {
				cat = &g.catalog.Entries[catID]
				inst.AnimationIndex = cat.AnimationIndex
				classifyInteraction(&inst, cat)
			}
			if g.objReader != nil {
				if e, err := g.objReader.Entry(catID); err == nil {
					inst.SpriteW = int(e.Width)
					inst.SpriteH = int(e.Height)
				}
			}
			// Rasterize the object's cube into the walkability grid
			// (re_docs/formats/collide.md, fcn.0056d720): rect origin
			// = world position + the collide record's anchor, u span
			// from x_extent, v span from width/2; the mask derives
			// from the object flags word. The cube `type` field plays
			// NO part in the movement path — fcn.00572100 gates the
			// rasterize call purely on the derived mask (0x5721ec:
			// stamp iff mask != 0 || height != 0), never on i16[6].
			// An earlier `cr.Type != 0` gate wrongly dropped the many
			// solid walls whose collide record has type 0 (e.g. the
			// tall stone walls id 4220/4221/4224, mask 0x009), letting
			// the player walk straight through them.
			if g.collide0 != nil && catID >= 0 && catID < len(g.collide0.Records) {
				cr := g.collide0.Records[catID]
				mask := objectMask(cat, &inst)
				// Keep a collider for any object that blocks now (mask
				// != 0) or that can start blocking on use (an open door
				// has mask 0 but must re-stamp when closed).
				if mask != 0 || inst.Interactive {
					cube := collision.Cube{
						X:       wx + int(cr.AnchorX),
						Y:       wy + int(cr.AnchorY),
						XExtent: max(int(cr.XExtent), 0),
						Width:   int(cr.Width),
						Mask:    mask,
					}
					g.walkGrid.Stamp(cube) // no-op when mask == 0
					hw := max(int(cr.Width)/2, 1)
					box := aabb{
						X: cube.X,
						Y: cube.Y - hw,
						W: max(cube.XExtent, 1),
						H: hw,
					}
					inst.ColliderIdx = len(g.colliders)
					g.colliders = append(g.colliders, collider{cube: cube, box: box})
				}
			}
			g.insts = append(g.insts, inst)
		}
	}); err != nil {
		return fmt.Errorf("walk world.x%d: %w", n, err)
	}

	sort.Slice(g.cells, func(i, j int) bool {
		if g.cells[i].CellY != g.cells[j].CellY {
			return g.cells[i].CellY < g.cells[j].CellY
		}
		return g.cells[i].CellX < g.cells[j].CellX
	})
	// Baseline emission order for the topological sort: Y (foot position),
	// then elevation. This fixes the draw order of NON-overlapping sprites
	// only — overlapping ones get explicit dependency edges from the
	// CSpriteSorter comparator port (render.go / sort.go,
	// re_docs/render-trace.md). Note FUN_004f7b40's `(65536 - in_Y) * 2` is
	// the FX-layer depth key, NOT the world-object sort (retracted in
	// render-trace.md); the engine's own baseline is its visible-cell-list
	// iteration order, which is undocumented — Y-then-elevation is our
	// heuristic stand-in.
	// Layer is a per-object ELEVATION (10-bit `& 0x3ff` in the world record),
	// not a draw-order priority, sorting by Layer first puts walls over floor
	// stains/decals incorrectly.
	sort.SliceStable(g.insts, func(i, j int) bool {
		if g.insts[i].Y != g.insts[j].Y {
			return g.insts[i].Y < g.insts[j].Y
		}
		return g.insts[i].Layer < g.insts[j].Layer
	})

	// Rebuild the mover against the fresh grid, keeping the player where
	// it stood (position carries across region switches).
	g.mover = mover.New(g.walkGrid, playerMask, g.player.X, g.player.Y, heroWalkSpeed)

	log.Printf("region %d: %d floor cells, %d object instances, %d colliders",
		n, len(g.cells), len(g.insts), len(g.colliders))
	return nil
}

// loadHeights fills the fresh walkability grid's terrain layer from
// static/height.x<n> (re_docs/cell-grid.md). It runs before the world's
// objects are stamped: [collision.Grid.Stamp] measures a walk-on cube's climb
// byte down from the base height, and the loader clears the climb bit on a
// cell the map blocks, which a later stamp puts back
// (TestLoadHeightsBeforeStamps).
//
// A missing or malformed file is fatal, as a missing world.x<n> is: 78,547 of
// region 0's walls live in it and nothing else supplies them.
func (g *Game) loadHeights(n int) error {
	path := fmt.Sprintf("%s/main/startup/static/height.x%d", g.gameDir, n)
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open height.x%d: %w", n, err)
	}
	defer f.Close()
	hm, err := height.Decode(f)
	if err != nil {
		return fmt.Errorf("height.x%d: %w", n, err)
	}
	blocked, climb := g.walkGrid.LoadHeights(hm)
	log.Printf("region %d: height map: %d cells blocked by the map, %d climb-gated", n, blocked, climb)
	return nil
}

// objectMask derives the instance's walkability-grid mask from its
// catalogue flags word and current open/locked state
// (re_docs/formats/collide.md; the engine re-derives it on every
// CObject::Use re-stamp).
func objectMask(cat *objects.Object, inst *objectInst) uint16 {
	if cat == nil {
		return collision.MaskStatic
	}
	door := cat.HasS(objects.SDoor)
	return collision.ObjectMask(collision.ObjectState{
		PlayerBlock:   cat.HasS(objects.SPlayerBlock),
		WalkThrough:   cat.HasS(objects.SWalkThrough),
		Door:          door,
		Closed:        door && !inst.Open,
		Locked:        inst.Locked,
		Light:         cat.HasS(objects.SLight),
		Lever:         cat.HasS(objects.SLever),
		WalkOn:        cat.HasSB(objects.SBWalkOn),
		NoLookThrough: cat.HasSB(objects.SBNoLookThrough),
	})
}

// classifyInteraction seeds the instance's interaction state from the
// catalogue entry, per the documented instance flags word
// (re_docs/object-interaction.md): the kind bits sb_door/sb_chest/sb_lever
// mark usable objects, sb_closed is the initial open/closed state, and
// sb_locked is the hard gate CObject::Use checks before opening. Only a
// door's blocker flips on use — a chest stays blocking while its lid opens.
//
// This replaces an earlier heuristic that classified by collide Type==2
// minus SBLightBlocker, which had no doc basis and misfired on walls.
func classifyInteraction(inst *objectInst, cat *objects.Object) {
	if cat.HasSB(objects.SBUseClass) {
		inst.Interactive = true
	}
	door := cat.HasS(objects.SDoor)
	chest := cat.HasS(objects.SChest)
	if door || chest || cat.HasS(objects.SLever) {
		inst.Interactive = true
	}
	inst.ToggleCollider = door
	if door || chest {
		inst.Open = !cat.HasS(objects.SClosed)
	}
	inst.Locked = cat.HasS(objects.SLocked)
}
