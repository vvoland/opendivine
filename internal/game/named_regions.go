// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"grono.dev/opendivine/pkg/assets/region"
)

// Named-region tables span worlds and are independent of world.x<n> loads.
// Keep the unverified parser opt-in so a bad table cannot prevent exploring.
func (g *Game) loadNamedRegions() {
	g.namedRegionsLoaded = true
	for n := range g.namedRegions {
		name := fmt.Sprintf("region.%03d", n)

		data, err := os.ReadFile(filepath.Join(g.gameDir, "global", name))
		if err != nil {
			log.Printf("named regions: %s: %v", name, err)
			continue
		}

		table, err := region.Decode(bytes.NewReader(data))
		if err != nil {
			log.Printf("named regions: %s: %v", name, err)
			continue
		}

		g.namedRegions[n] = table
		log.Printf("named regions: %s: %s, %d records", name, table.Tag, len(table.Records))
	}
}

type regionOverlay struct {
	tag            region.SubTag
	index          int
	record         *region.Record
	containsPlayer bool
}

func (g *Game) regionOverlays() []regionOverlay {
	var overlays []regionOverlay

	for _, table := range g.namedRegions {
		if table == nil {
			continue
		}

		for i := range table.Records {
			rec := &table.Records[i]
			if rec.WorldID != uint32(g.region) || len(rec.Vertices) < 2 {
				continue
			}

			overlays = append(overlays, regionOverlay{
				tag: table.Tag, index: i, record: rec,
				containsPlayer: rec.Contains(g.player.X, g.player.Y),
			})
		}
	}

	return overlays
}

func (g *Game) drawNamedRegions(screen *ebiten.Image, worldToScreen func(float64, float64) (float64, float64)) string {
	loaded := 0

	for _, table := range g.namedRegions {
		if table != nil {
			loaded++
		}
	}

	msg := &bytes.Buffer{}
	fmt.Fprintf(msg, "\nregions[F11]: %d/%d tables; cyan outlines, yellow at player", loaded, len(g.namedRegions))

	if loaded != len(g.namedRegions) {
		fmt.Fprint(msg, " (load errors: see log)")
	}

	matches := 0

	for _, overlay := range g.regionOverlays() {
		col := color.NRGBA{R: 0x30, G: 0xc0, B: 0xff, A: 0xc0}
		width := float32(1)

		if overlay.containsPlayer {
			col = color.NRGBA{R: 0xff, G: 0xe0, B: 0x30, A: 0xff}
			width = 2

			fmt.Fprintf(msg, "\n  %s #%d %q", overlay.tag, overlay.index, overlay.record.Name)

			matches++
		}

		vertices := overlay.record.Vertices

		ax, ay := worldToScreen(vertices[len(vertices)-1].X, vertices[len(vertices)-1].Y)
		for _, v := range vertices {
			bx, by := worldToScreen(v.X, v.Y)
			// Most subsystem polygons are far outside the viewport.
			if math.Max(ax, bx) >= 0 && math.Min(ax, bx) <= float64(g.winW) &&
				math.Max(ay, by) >= 0 && math.Min(ay, by) <= float64(g.winH) {
				vector.StrokeLine(screen, float32(ax), float32(ay), float32(bx), float32(by), width, col, false)
			}

			ax, ay = bx, by
		}
	}

	if matches == 0 {
		fmt.Fprint(msg, "\n  no containing polygons in loaded tables")
	}

	return msg.String()
}
