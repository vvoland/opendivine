// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

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
	g.colliderGrid = map[int][]int32{}
	g.floorTiles = map[int16]*ebiten.Image{}
	g.objSprites = map[int]*sprite{}

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
				X:           wx,
				Y:           wy,
				ObjID:       catID,
				Layer:       int(o.Layer),
				Elev:        int(o.Layer),
				ColliderIdx: -1,
			}
			var cat *objects.Object
			if g.catalog != nil && catID >= 0 && catID < len(g.catalog.Entries) {
				cat = &g.catalog.Entries[catID]
				inst.Interactive, inst.ToggleCollider = objectInteractionFlags(cat, 0)
			}
			if g.objReader != nil {
				if e, err := g.objReader.Entry(catID); err == nil {
					inst.SpriteW = int(e.Width)
					inst.SpriteH = int(e.Height)
				}
			}
			// Build collision rect for blocker objects.
			// Type=1 - static obstacle,
			// Type=2 - interactive (door, chest): blocks while closed, but the
			// player can open it to pass (see useObject / tryInteract).
			// Width-zero / no-Z entries (decals, ground stains) don't block.
			if g.collide0 != nil && catID < len(g.collide0.Records) {
				cr := g.collide0.Records[catID]
				if cr.Type != 0 && cr.ZHeight > 0 && cr.Width > 0 {
					hw := max(int(cr.Width)/2, 1)
					box := aabb{X: wx - hw, Y: wy - hw, W: hw * 2, H: hw * 2}
					inst.ColliderIdx = len(g.colliders)
					interactive, toggleCollider := objectInteractionFlags(cat, cr.Type)
					inst.Interactive = inst.Interactive || interactive
					inst.ToggleCollider = inst.ToggleCollider || toggleCollider
					g.colliders = append(g.colliders, collider{box: box, enabled: true})
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
	// Painter's-algorithm sort by Y (foot position) only.
	// The engine's depth key is `out_Z = (65536 - in_Y) * 2` from
	// `FUN_004f7b40`, i.e. depth = -Y.
	// Layer is a per-object ELEVATION (10-bit `& 0x3ff` in the world record),
	// not a draw-order priority, sorting by Layer first puts walls over floor
	// stains/decals incorrectly.
	// For ties at the same Y, fall back to elevation so a flask-on-table draws
	// after the table.
	sort.SliceStable(g.insts, func(i, j int) bool {
		if g.insts[i].Y != g.insts[j].Y {
			return g.insts[i].Y < g.insts[j].Y
		}
		return g.insts[i].Layer < g.insts[j].Layer
	})

	// Bucket colliders into 64x64-px cells for fast spatial queries.
	for i := range g.colliders {
		c := g.colliders[i].box
		minCX := c.X / cellPx
		maxCX := (c.X + c.W - 1) / cellPx
		minCY := c.Y / cellPx
		maxCY := (c.Y + c.H - 1) / cellPx
		for cy := minCY; cy <= maxCY; cy++ {
			for cx := minCX; cx <= maxCX; cx++ {
				if cx < 0 || cy < 0 || cx >= worldCellsX || cy >= worldCellsY {
					continue
				}
				k := cy*worldCellsX + cx
				g.colliderGrid[k] = append(g.colliderGrid[k], int32(i))
			}
		}
	}
	log.Printf("region %d: %d floor cells, %d object instances, %d colliders, %d grid buckets",
		n, len(g.cells), len(g.insts), len(g.colliders), len(g.colliderGrid))
	return nil
}

func objectInteractionFlags(cat *objects.Object, collideType int16) (interactive, toggleCollider bool) {
	if cat != nil && cat.HasSB(objects.SBUseClass) {
		interactive = true
	}
	if collideType == 2 && (cat == nil || !cat.HasSB(objects.SBLightBlocker)) {
		interactive = true
		toggleCollider = true
	}
	return interactive, toggleCollider
}
