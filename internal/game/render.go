// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"grono.dev/opendivine/internal/game/character"
)

func (g *Game) Draw(screen *ebiten.Image) {
	// Visible world rectangle in native pixels.
	halfW := float64(g.winW) / 2.0 / g.zoom
	halfH := float64(g.winH) / 2.0 / g.zoom
	viewMinX := g.camX - halfW
	viewMinY := g.camY - halfH
	viewMaxX := g.camX + halfW
	viewMaxY := g.camY + halfH

	worldToScreen := func(wx, wy float64) (float64, float64) {
		return (wx-g.camX)*g.zoom + float64(g.winW)/2.0,
			(wy-g.camY)*g.zoom + float64(g.winH)/2.0
	}

	// Floor tiles, cells sorted by (Y, X), binary-search first visible row.
	floorDrawn := 0
	firstRow := sort.Search(len(g.cells), func(i int) bool {
		return float64(g.cells[i].CellY)+cellPx > viewMinY
	})
	for i := firstRow; i < len(g.cells) && g.showFloors; i++ {
		c := g.cells[i]
		cy := float64(c.CellY)
		if cy > viewMaxY {
			break
		}
		cx := float64(c.CellX)
		if cx+cellPx < viewMinX || cx > viewMaxX {
			continue
		}
		sx, sy := worldToScreen(cx, cy)
		// Tile 0 is dirt (not void); DO draw it.
		if c.FloorID != tileVoid {
			if tile := g.floorTile(c.FloorID); tile != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(g.zoom, g.zoom)
				op.GeoM.Translate(sx, sy)
				screen.DrawImage(tile, op)
				floorDrawn++
			}
		}
		// Overlay tile (secondary 64×64 tile from imagelist 2, roads, stains,
		// decals) drawn on top of the floor. -1 = none.
		if c.OverlayID >= 0 {
			if otile := g.floorTile(c.OverlayID); otile != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(g.zoom, g.zoom)
				op.GeoM.Translate(sx, sy)
				screen.DrawImage(otile, op)
			}
		}
	}

	// Objects, already sorted by (Layer, Y).
	//
	// Engine-traced render formula (div.exe:0x004a3070 + FUN_005830c0):
	//
	//   tlY = worldY - layer
	//
	// The 10-bit Layer field in world.x<n>'s ord_kind encodes the
	// cumulative pixel elevation of the object:
	// 0 for floor-level objects,
	// +z_height per stacked SBPutOn surface (table=41, shelf, etc), and the
	// on-top item's own offset.
	//
	// Objects + player are drawn together with painter's-algorithm Y-sort so
	// walls in front of the player hide his lower half and walls behind don't.
	//
	// Player sort key = (Layer=0, playerY) matches floor-level objects. Build
	// the list of characters to Y-sort with world objects. Currently just the
	// player; once NPCs are spawned they go in here too. Each character's sort
	// key = (Layer=0, Y).
	type charDraw struct {
		c    *character.Character
		done bool
	}
	chars := []charDraw{{c: g.player}}

	objDrawn := 0

	// drawInst emits a single objectInst (only if the sprite decoded), shared
	// between the back-emit pass and the topo pass.
	// Returns the world Y the engine would use for ordering (used by character
	// interleave).
	drawInst := func(in objectInst) {
		spr := g.objectSprite(in.ObjID)
		if spr == nil {
			return
		}
		w := float64(spr.img.Bounds().Dx())
		h := float64(spr.img.Bounds().Dy())
		tlX := float64(in.X)
		tlY := float64(in.Y - in.Elev)
		if tlX+w < viewMinX || tlX > viewMaxX || tlY+h < viewMinY || tlY > viewMaxY {
			return
		}
		sx, sy := worldToScreen(tlX, tlY)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(g.zoom, g.zoom)
		op.GeoM.Translate(sx, sy)
		if in.Open {
			// Placeholder for the engine's open-frame swap: fade an opened
			// door/chest so its state is visible until the open animation
			// frame is wired (re_docs/object-interaction.md).
			op.ColorScale.ScaleAlpha(0.35)
		}
		screen.DrawImage(spr.img, op)
		objDrawn++
	}

	if g.showObjects {
		// Per-frame depth-sort following CSpriteSorter::Render
		// (div.exe:0x00547000): topological pass over the visible sprites with
		// overlap-gated pairwise compares (sort.go).
		//
		// 1. Cull to visible instances.
		// 2. Build a 7-int sort record per visible sprite from its
		//    collide.<n> + world (X,Y,layer) data.
		// 3. For each pair of overlapping AABBs run FUN_00546e40 to create
		// directed edges; DFS-emit.
		//
		// The engine's back-emit pass at z=30000 (FUN_00547000 lines
		// 0x00547042..0x00547080) is exclusively for blood decals laying flat
		// after their animation ends.
		// We do not implement gore yet, so all visible sprites flow through the
		// topo sort.
		//
		// Characters are queued INTO the same sort list (the engine adds them
		// as ordinary sprites sprite sorter.
		// Synthesise a per-character cube: the foot is at the world (X, Y), the
		// cube is a small footprint (matches the 12-px collision cube in
		// playerBlocked), and ZHeight is the composite-frame full vertical
		// extent so the bbox-mid r[3] places the character correctly relative
		// to walls / trees.
		visible := make([]sortRecord, 0, 256)

		// Append characters first so they participate in the topo sort.
		// Synthetic cube: foot at world (X,Y), 12-px square footprint (matches
		// playerBlocked), height = CompMaxHeight (full sprite vertical extent).
		// Layer = 0.
		for ci := range chars {
			c := chars[ci].c
			if c == nil {
				continue
			}
			const cubeHalfW = 6
			zH := c.CompMaxHeight
			if zH <= 0 {
				zH = 32
			}
			tlX := c.X - float64(c.ClassAnchorX)
			tlY := c.Y - float64(c.ClassAnchorY)
			brX := tlX + float64(c.ClassAnchorX*2)
			brY := tlY + float64(c.CompMaxHeight)
			if brX < viewMinX || tlX > viewMaxX || brY < viewMinY || tlY > viewMaxY {
				continue
			}
			// Engine formulas with AnchorX=AnchorY=0:
			//   imgRec[+0x10] = 0 + cubeW/2 + worldX = wX + cubeHalfW
			//   imgRec[+0x12] = 0 - cubeW/2 + worldY = wY - cubeHalfW
			isoX := cubeHalfW + int(c.X)
			topY := -cubeHalfW + int(c.Y)
			rec := sortRecord{inst: -(ci + 1)}
			rec.r[0] = isoX + topY
			rec.r[1] = (cubeHalfW * 2) + rec.r[0]
			rec.r[2] = topY
			rec.r[3] = cubeHalfW + topY
			rec.r[4] = 0
			rec.r[5] = zH
			rec.r[6] = -(ci + 1)
			rec.bboxX1 = int(tlX)
			rec.bboxY1 = int(tlY)
			rec.bboxX2 = int(brX)
			rec.bboxY2 = int(brY)
			visible = append(visible, rec)
		}

		for idx := range g.insts {
			in := &g.insts[idx]
			spr := g.objectSprite(in.ObjID)
			if spr == nil {
				continue
			}
			w := spr.img.Bounds().Dx()
			h := spr.img.Bounds().Dy()
			tlX := float64(in.X)
			tlY := float64(in.Y - in.Elev)
			if tlX+float64(w) < viewMinX || tlX > viewMaxX ||
				tlY+float64(h) < viewMinY || tlY > viewMaxY {
				continue
			}
			// Pull AnchorX/AnchorY/XExtent/ZHeight/Width from collide.0.
			// Width here is the cube's Y-axis extent (used for bbox
			// mid-Y), AnchorX/Y are the sprite-local foot offsets.
			var anchorX, anchorY, xExtent, zHeight, cubeW int
			if g.collide0 != nil && in.ObjID >= 0 && in.ObjID < len(g.collide0.Records) {
				cr := g.collide0.Records[in.ObjID]
				anchorX = int(cr.AnchorX)
				anchorY = int(cr.AnchorY)
				xExtent = int(cr.XExtent)
				zHeight = int(cr.ZHeight)
				cubeW = int(cr.Width)
			}
			rec := sortRecord{inst: idx}
			// Per FUN_0059c7e0 at div.exe:0x0059c9c2..0x0059c9d4 the
			// runtime mirrors are imgRec[+0x10]=AnchorX+W/2+worldX,
			// imgRec[+0x12]=AnchorY-W/2+worldY, imgRec[+0x14]=Z.
			isoX := anchorX + cubeW/2 + in.X
			topY := anchorY - cubeW/2 + in.Y
			rec.r[0] = isoX + topY
			rec.r[1] = xExtent + rec.r[0]
			rec.r[2] = topY
			rec.r[3] = cubeW/2 + topY
			rec.r[4] = in.Layer
			rec.r[5] = zHeight
			rec.r[6] = idx
			// World AABB used by the overlap gate.  We use sprite
			// extent (top-left + W/H in our render-model coords).
			rec.bboxX1 = int(tlX)
			rec.bboxY1 = int(tlY)
			rec.bboxX2 = int(tlX) + w
			rec.bboxY2 = int(tlY) + h
			visible = append(visible, rec)
		}

		// Build edges and DFS-emit.
		for i := range visible {
			a := &visible[i]
			for j := range visible {
				if i == j {
					continue
				}
				b := &visible[j]
				if !aabbOverlap(a.bboxX1, a.bboxY1, a.bboxX2, a.bboxY2,
					b.bboxX1, b.bboxY1, b.bboxX2, b.bboxY2) {
					continue
				}
				// FUN_00546e40 returns +1 if a is in front, -1 if b is in
				// front.
				// FUN_00546ec0 visits a node's deps BEFORE blitting it, so deps
				// are drawn first (behind).
				// If a is in front of b, then b is behind a, so a depends on b,
				// visit b first when emitting a, so a draws on top of b.
				if compareSortRecords(a, b) == 1 {
					a.deps = append(a.deps, j)
				}
			}
		}

		// DFS-emit (FUN_00546ec0). Visit deps before emitting the
		// node; "visited" guards against cycles.
		var emit func(int)
		emit = func(k int) {
			v := &visible[k]
			if v.visited {
				return
			}
			v.visited = true
			for _, d := range v.deps {
				emit(d)
			}
			if v.inst < 0 {
				ci := -v.inst - 1
				if ci >= 0 && ci < len(chars) && chars[ci].c != nil && !chars[ci].done {
					chars[ci].c.Draw(screen, g.zoom, worldToScreen)
					chars[ci].done = true
				}
				return
			}
			drawInst(g.insts[v.inst])
		}
		for k := range visible {
			emit(k)
		}
	}
	// Catch any characters whose Y was past every object, draw them after the
	// world.
	// Falls back to a cyan diamond if the character has no sprite layers (load
	// failure).
	for i := range chars {
		if chars[i].done || chars[i].c == nil {
			continue
		}
		if !chars[i].c.Draw(screen, g.zoom, worldToScreen) {
			px, py := worldToScreen(chars[i].c.X, chars[i].c.Y)
			size := 8.0 * g.zoom
			c := color.NRGBA{R: 0x00, G: 0xff, B: 0xff, A: 0xff}
			vector.StrokeLine(screen, float32(px), float32(py-size), float32(px+size), float32(py), 1, c, false)
			vector.StrokeLine(screen, float32(px+size), float32(py), float32(px), float32(py+size), 1, c, false)
			vector.StrokeLine(screen, float32(px), float32(py+size), float32(px-size), float32(py), 1, c, false)
			vector.StrokeLine(screen, float32(px-size), float32(py), float32(px), float32(py-size), 1, c, false)
		}
	}

	// Click-to-walk target, small green crosshair, drawn on top.
	if g.hasDest {
		dx, dy := worldToScreen(g.destX, g.destY)
		size := 5.0 * g.zoom
		c := color.NRGBA{R: 0x00, G: 0xff, B: 0x00, A: 0xff}
		vector.StrokeLine(screen, float32(dx-size), float32(dy), float32(dx+size), float32(dy), 1, c, false)
		vector.StrokeLine(screen, float32(dx), float32(dy-size), float32(dx), float32(dy+size), 1, c, false)
	}

	msg := bytes.NewBufferString("")
	follow := "follow"
	if !g.cameraFollow {
		follow = "free"
	}
	fmt.Fprintf(msg, "region %d  cam (%.0f, %.0f)  player (%.0f, %.0f) %s[F9]  zoom %.2fx  %.1f fps",
		g.region, g.camX, g.camY, g.player.X, g.player.Y, follow, g.zoom, ebiten.ActualFPS())

	// Hover info: world (X,Y), cell (CX,CY), and the topmost object instance
	// whose foot is closest to the cursor (within a small pixel-radius). No RE
	// claim is bet on; just lookups against the already-loaded world.x* and
	// objects.000 catalog.
	mx, my := ebiten.CursorPosition()
	hoverWX := (float64(mx)-float64(g.winW)/2.0)/g.zoom + g.camX
	hoverWY := (float64(my)-float64(g.winH)/2.0)/g.zoom + g.camY
	hoverCX := int(hoverWX) / cellPx
	hoverCY := int(hoverWY) / cellPx
	fmt.Fprintf(msg, "\nhover  world (%.0f, %.0f)  cell (%d, %d)",
		hoverWX, hoverWY, hoverCX, hoverCY)

	// Find the closest object foot within ~24 px. Cheap O(N) over only the
	// visible insts is fine at typical view counts.
	closest := -1
	closestD2 := 24.0 * 24.0
	for i := range g.insts {
		dx := float64(g.insts[i].X) - hoverWX
		dy := float64(g.insts[i].Y) - hoverWY
		d2 := dx*dx + dy*dy
		if d2 < closestD2 {
			closestD2 = d2
			closest = i
		}
	}
	if closest >= 0 {
		in := g.insts[closest]
		name := ""
		sb := uint32(0)
		if g.catalog != nil && in.ObjID >= 0 && in.ObjID < len(g.catalog.Entries) {
			e := g.catalog.Entries[in.ObjID]
			name = e.Name
			sb = e.SBFlags
		}
		fmt.Fprintf(msg, "\n  obj id=%d %q  layer=%d  sb=0x%05x", in.ObjID, name, in.Layer, sb)
		if in.Interactive {
			state := "closed"
			if in.Open {
				state = "open"
			}
			fmt.Fprintf(msg, "  [interactive: %s]", state)
		}
	}

	ebitenutil.DebugPrint(screen, msg.String())

	if g.wantShot {
		g.wantShot = false
		path := fmt.Sprintf("screenshots/divine-%s.png", time.Now().Format("20060102-150405"))
		if err := saveScreenshot(screen, path); err != nil {
			log.Printf("screenshot: %v", err)
		} else {
			log.Printf("screenshot: %s", path)
		}
	}
	if g.shotPath != "" {
		if err := saveScreenshot(screen, g.shotPath); err != nil {
			log.Fatalf("screenshot: %v", err)
		}
		log.Printf("screenshot: %s", g.shotPath)
		os.Exit(0)
	}
}

// saveScreenshot reads the current framebuffer and writes it as PNG to path.
func saveScreenshot(screen *ebiten.Image, path string) error {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	buf := make([]byte, w*h*4)
	screen.ReadPixels(buf)
	img := &image.RGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
