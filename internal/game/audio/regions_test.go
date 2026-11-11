// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"errors"
	"math"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"grono.dev/opendivine/pkg/assets/musicdat"
	"grono.dev/opendivine/pkg/assets/region"
)

type musicPlayback struct {
	label  string
	loop   bool
	volume float64
}

type recordingMusicPlayer struct {
	plays []musicPlayback
	stops int
	err   error
}

func (p *recordingMusicPlayer) PlayMusic(label string, loop bool, volume float64) error {
	p.plays = append(p.plays, musicPlayback{label, loop, volume})
	return p.err
}

func (p *recordingMusicPlayer) Stop() { p.stops++ }

// Zone changes, not frames or camera movement, drive playback. Overlapping
// polygons use file order, worlds stay separate and gains are not pick weights.
func TestRegionMusicTransitions(t *testing.T) {
	t.Parallel()

	square := []region.Vertex{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	otherSquare := []region.Vertex{{X: 20, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 10}, {X: 20, Y: 10}}
	m := &MusicManager{zones: []musicdat.Zone{
		{Label: "A", Lists: [5][]musicdat.Assignment{
			{{Label: "quiet", Volume: 0}, {Label: "loud", Volume: 0.8}},
		}},
		{Label: "B", Lists: [5][]musicdat.Assignment{{{Label: "other", Volume: 0.3}}}},
		{Label: "C", Lists: [5][]musicdat.Assignment{{{Label: "world1", Volume: 0.5}}}},
		{Label: "ambient only", Lists: [5][]musicdat.Assignment{
			3: {{Label: "day", Volume: 1}}, 4: {{Label: "night", Volume: 1}},
		}},
	}}
	table := &region.File{Tag: region.SubTagMusic, Records: []region.Record{
		{Name: "C", WorldID: 1, Vertices: square},
		{Name: "A", WorldID: 0, Vertices: square},
		{Name: "B", WorldID: 0, Vertices: square},
		{Name: "B", WorldID: 0, Vertices: otherSquare},
		{Name: "ambient only", WorldID: 2, Vertices: square},
		{Name: "unbound", WorldID: 3, Vertices: square},
	}}
	r, err := NewRegionMusic(m, table)
	assert.NilError(t, err)

	player := &recordingMusicPlayer{}
	r.player = player

	assert.NilError(t, r.Update(0, 5, 5))
	assert.Equal(t, len(player.plays), 1)
	assert.Equal(t, player.plays[0].loop, true)

	choices := m.zones[0].Lists[0]
	assert.Assert(t, is.Contains(choices, musicdat.Assignment{
		Label: player.plays[0].label, Volume: float32(player.plays[0].volume),
	}))
	assert.NilError(t, r.Update(0, 6, 6))
	assert.Equal(t, len(player.plays), 1)
	assert.Equal(t, player.stops, 1)

	assert.NilError(t, r.Update(1, 5, 5))
	assert.Equal(t, player.plays[1], musicPlayback{"world1", true, 0.5})
	assert.NilError(t, r.Update(0, 25, 5))
	assert.Equal(t, player.plays[2], musicPlayback{"other", true, float64(float32(0.3))})

	assert.NilError(t, r.Update(2, 5, 5))
	assert.Equal(t, len(player.plays), 3, "ambient lists must not be played as primary music")
	assert.NilError(t, r.Update(3, 5, 5))
	assert.Equal(t, len(player.plays), 3, "an unknown binding must stop playback")
	assert.NilError(t, r.Update(0, 100, 100))
	assert.Equal(t, len(player.plays), 3)
	assert.Equal(t, player.stops, 6)
	assert.NilError(t, r.Update(0, 101, 101))
	assert.Equal(t, player.stops, 6)

	assert.NilError(t, r.Update(0, 5, 5))
	assert.Equal(t, len(player.plays), 4, "re-entry must start music again")
}

// A corrupt or missing clip must not trigger archive reads and log spam every
// frame, but leaving and returning must allow another playback attempt.
func TestRegionMusicRetriesAfterTransition(t *testing.T) {
	t.Parallel()

	table := &region.File{Tag: region.SubTagMusic, Records: []region.Record{
		{Name: "A", Vertices: []region.Vertex{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}},
	}}
	m := &MusicManager{zones: []musicdat.Zone{
		{Label: "A", Lists: [5][]musicdat.Assignment{{{Label: "track", Volume: 1}}}},
	}}
	r, err := NewRegionMusic(m, table)
	assert.NilError(t, err)
	player := &recordingMusicPlayer{err: errors.New("missing clip")}
	r.player = player

	assert.ErrorContains(t, r.Update(0, 1, 1), "missing clip")
	assert.NilError(t, r.Update(0, 2, 2))
	assert.Equal(t, len(player.plays), 1)
	assert.Equal(t, player.stops, 1)

	player.err = nil

	assert.NilError(t, r.Update(0, 20, 20))
	assert.NilError(t, r.Update(0, 1, 1))
	assert.Equal(t, len(player.plays), 2)
}

func TestRegionMusicRequiresMusicTable(t *testing.T) {
	t.Parallel()

	_, err := NewRegionMusic(&MusicManager{}, &region.File{Tag: region.SubTagStory})
	assert.ErrorContains(t, err, "MusicV1.0")
}

// Reject invalid gains before reading an archive or reaching the audio backend.
func TestPlayMusicRejectsInvalidVolume(t *testing.T) {
	t.Parallel()

	m := &MusicManager{}
	for _, volume := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		assert.ErrorContains(t, m.PlayMusic("track", true, volume), "invalid music volume")
	}
}
