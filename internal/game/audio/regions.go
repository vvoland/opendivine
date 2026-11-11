// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"errors"
	"fmt"
	"log"
	"math/rand/v2"

	"grono.dev/opendivine/pkg/assets/musicdat"
	"grono.dev/opendivine/pkg/assets/region"
)

type musicPlayer interface {
	PlayMusic(label string, loop bool, volume float64) error
	Stop()
}

// RegionMusic selects primary music from the first polygon containing the
// player in the active world.
// It does not implement fades, timed overrides or day/night ambients.
type RegionMusic struct {
	player  musicPlayer
	regions *region.File
	zones   map[string][]musicdat.Assignment

	initialized bool
	worldID     uint32
	zone        string
}

// NewRegionMusic connects a music manager to a MusicV1.0 region table.
func NewRegionMusic(m *MusicManager, regions *region.File) (*RegionMusic, error) {
	if regions == nil || regions.Tag != region.SubTagMusic {
		return nil, errors.New("music: expected a MusicV1.0 region table")
	}

	r := &RegionMusic{
		player: m, regions: regions,
		zones: make(map[string][]musicdat.Assignment, len(m.zones)),
	}
	for _, zone := range m.zones {
		r.zones[zone.Label] = zone.Lists[0]
	}

	return r, nil
}

// Update changes tracks only when the player's zone or world changes.
// One uniformly selected L0 entry loops until the next transition; its float
// is playback volume, not a selection weight.
// This is a port policy, not the engine's full playlist/silence state machine.
// An unbound zone or empty music list stops playback.
// Failed playback is retried only after another transition, not every tick.
func (r *RegionMusic) Update(worldID uint32, x, y float64) error {
	name := ""

	for i := range r.regions.Records {
		rec := &r.regions.Records[i]
		if rec.WorldID == worldID && rec.Contains(x, y) {
			name = rec.Name
			break
		}
	}

	if r.initialized && r.worldID == worldID && r.zone == name {
		return nil
	}

	r.initialized, r.worldID, r.zone = true, worldID, name
	r.player.Stop()

	entries := r.zones[name]
	if len(entries) == 0 {
		log.Printf("music zone %q: no primary music", name)
		return nil
	}

	entry := entries[rand.IntN(len(entries))]
	if err := r.player.PlayMusic(entry.Label, true, float64(entry.Volume)); err != nil {
		return fmt.Errorf("music zone %q track %q: %w", name, entry.Label, err)
	}

	log.Printf("music zone %q: playing %q (volume %.2f)", name, entry.Label, entry.Volume)

	return nil
}
