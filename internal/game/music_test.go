// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"testing"

	"gotest.tools/v3/assert"

	"grono.dev/opendivine/internal/game/audio"
	"grono.dev/opendivine/internal/game/character"
	"grono.dev/opendivine/pkg/assets/region"
)

// Music must work without F11 and reuse the same decoded table as the overlay,
// including when the overlay later loads the other seven subsystem tables.
func TestRegionMusicDoesNotRequireOverlay(t *testing.T) {
	t.Parallel()

	g := &Game{gameDir: t.TempDir(), player: &character.Character{}}
	table := &region.File{Tag: region.SubTagMusic}
	g.namedRegions[1] = table
	g.startRegionMusic(&audio.MusicManager{})
	assert.Assert(t, g.music != nil)
	assert.Equal(t, g.showRegions, false)
	assert.Equal(t, g.namedRegionsLoaded, false)

	g.loadNamedRegions()
	assert.Equal(t, g.namedRegions[1], table)
	assert.Equal(t, g.namedRegionsLoaded, true)
	g.updateRegionMusic()
}
