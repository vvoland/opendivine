// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"grono.dev/opendivine/internal/game/character"
	"grono.dev/opendivine/pkg/assets/region"
)

// World switching must change both the outlined polygons and containment
// labels, even when two worlds have polygons at identical coordinates.
func TestRegionOverlaysFollowWorldAndPlayer(t *testing.T) {
	t.Parallel()

	square := []region.Vertex{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	g := &Game{player: &character.Character{X: 5, Y: 5}}
	g.namedRegions[0] = &region.File{
		Tag: region.SubTagStory,
		Records: []region.Record{
			{Name: "story", WorldID: 0, Vertices: square},
			{Name: "other world", WorldID: 1, Vertices: square},
			{Name: "empty", WorldID: 0},
		},
	}
	g.namedRegions[1] = &region.File{
		Tag:     region.SubTagMusic,
		Records: []region.Record{{Name: "music", WorldID: 0, Vertices: square}},
	}

	overlays := g.regionOverlays()
	assert.Equal(t, len(overlays), 2)
	assert.Equal(t, overlays[0].tag, region.SubTagStory)
	assert.Equal(t, overlays[0].index, 0)
	assert.Equal(t, overlays[0].record.Name, "story")
	assert.Equal(t, overlays[0].containsPlayer, true)
	assert.Equal(t, overlays[1].tag, region.SubTagMusic)
	assert.Equal(t, overlays[1].containsPlayer, true)

	g.player.X = 15
	for _, overlay := range g.regionOverlays() {
		assert.Equal(t, overlay.containsPlayer, false)
	}

	g.player.X = 5
	g.region = 1
	overlays = g.regionOverlays()
	assert.Equal(t, len(overlays), 1)
	assert.Equal(t, overlays[0].index, 1)
	assert.Equal(t, overlays[0].record.Name, "other world")
	assert.Equal(t, overlays[0].containsPlayer, true)

	g.region = 2
	assert.Equal(t, len(g.regionOverlays()), 0)
}

// A missing or malformed debug table must not discard the tables that loaded.
func TestLoadNamedRegionsKeepsValidTables(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	global := filepath.Join(dir, "global")
	assert.NilError(t, os.MkdirAll(global, 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(global, "region.000"), []byte("bad magic"), 0o644))

	data := []byte("Divinity RegionsV1.0\x00")
	data = binary.LittleEndian.AppendUint32(data, 10)
	data = append(data, "MusicV1.0\x00"...)
	data = binary.LittleEndian.AppendUint32(data, 0)
	assert.NilError(t, os.WriteFile(filepath.Join(global, "region.001"), data, 0o644))

	g := &Game{gameDir: dir}
	g.loadNamedRegions()
	assert.Equal(t, g.namedRegionsLoaded, true)
	assert.Assert(t, g.namedRegions[1] != nil)
	assert.Equal(t, g.namedRegions[1].Tag, region.SubTagMusic)

	for n, table := range g.namedRegions {
		if n != 1 {
			assert.Assert(t, table == nil, "unexpected table %d", n)
		}
	}
}
