// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"

	"grono.dev/opendivine/pkg/assets/musicdat"
	"grono.dev/opendivine/pkg/assets/region"
)

type musicPlayer interface {
	PlayMusic(label string, loop bool, volume float64) error
	SetVolume(volume float64) error
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
	track       string
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
// Keep the current track if the new zone permits it, adjusting its volume.
// Otherwise, select one L0 entry uniformly and loop it.
// Each entry's float is playback volume, not a selection weight.
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

	label := musicZoneLabel(name)

	entries, found := r.zones[label]
	if !found {
		r.player.Stop()
		r.track = ""

		log.Printf("music region %q: no music.dat binding %q", name, label)
		return nil
	}

	if len(entries) == 0 {
		r.player.Stop()
		r.track = ""

		log.Printf("music zone %q: empty primary music list", name)
		return nil
	}

	for _, entry := range entries {
		if r.track != "" && entry.Label == r.track {
			if err := r.player.SetVolume(float64(entry.Volume)); err != nil {
				return fmt.Errorf("music zone %q track %q volume: %w", name, entry.Label, err)
			}

			log.Printf("music zone %q: continuing %q (volume %.2f)", name, entry.Label, entry.Volume)

			return nil
		}
	}

	r.player.Stop()
	r.track = ""

	entry := entries[rand.IntN(len(entries))]
	if err := r.player.PlayMusic(entry.Label, true, float64(entry.Volume)); err != nil {
		return fmt.Errorf("music zone %q track %q: %w", name, entry.Label, err)
	}

	r.track = entry.Label
	log.Printf("music zone %q: playing %q (volume %.2f)", name, entry.Label, entry.Volume)

	return nil
}

// The music database replaces each non-identifier character in polygon names
// with an underscore: "Aleroth Woods (Dark)" becomes "Aleroth_Woods__Dark_".
// Repeated underscores are significant and must not be collapsed.
func musicZoneLabel(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}

		return '_'
	}, name)
}
