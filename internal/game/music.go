// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"log"

	"grono.dev/opendivine/internal/game/audio"
)

func (g *Game) startRegionMusic(music *audio.MusicManager) {
	if music == nil {
		return
	}

	music.Stop()

	if err := g.loadNamedRegion(1); err != nil {
		log.Printf("world music: %v (continuing silently)", err)
		return
	}

	var err error

	g.music, err = audio.NewRegionMusic(music, g.namedRegions[1])
	if err != nil {
		log.Printf("world music: %v (continuing silently)", err)
		return
	}

	g.updateRegionMusic()
}

func (g *Game) updateRegionMusic() {
	if g.music == nil {
		return
	}

	if err := g.music.Update(uint32(g.region), g.player.X, g.player.Y); err != nil {
		log.Printf("world music: %v", err)
	}
}
