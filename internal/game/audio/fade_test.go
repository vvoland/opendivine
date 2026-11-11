// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

type recordingMusicStream struct {
	volume float64
	stops  int
}

func (s *recordingMusicStream) Play()                {}
func (s *recordingMusicStream) SetVolume(v float64)  { s.volume = v }
func (s *recordingMusicStream) PauseAndStopReading() { s.stops++ }

// Gain changes must take time, crossfades must overlap, and retiring streams
// must stop only after becoming silent. No audio device or wall clock is used.
func TestMusicFades(t *testing.T) {
	t.Parallel()

	first := &recordingMusicStream{}
	a := &musicVoice{label: "A", loop: true, player: first}
	m := &MusicManager{voices: []*musicVoice{a}}
	assert.NilError(t, m.PlayMusic("A", true, 0.75))
	assert.Equal(t, first.volume, 0.0)
	m.Update(time.Second)
	assert.Equal(t, first.volume, 0.375)
	m.Update(time.Second)
	assert.Equal(t, first.volume, 0.75)

	second := &recordingMusicStream{}
	b := &musicVoice{label: "B", loop: true, player: second}
	m.voices = append(m.voices, b)
	assert.NilError(t, m.PlayMusic("B", true, 0.25))
	assert.Equal(t, first.volume, 0.75, "a transition must not cut the outgoing track")
	assert.Equal(t, second.volume, 0.0)
	m.Update(time.Second)
	assert.Equal(t, first.volume, 0.375)
	assert.Equal(t, second.volume, 0.125)
	assert.Equal(t, first.stops, 0)
	m.Update(time.Second)
	assert.Equal(t, first.volume, 0.0)
	assert.Equal(t, first.stops, 1)
	assert.Equal(t, second.volume, 0.25)
	assert.Equal(t, len(m.voices), 1)

	assert.NilError(t, m.SetVolume(0.75))
	assert.Equal(t, second.volume, 0.25)
	m.Update(time.Second)
	assert.Equal(t, second.volume, 0.5)
	m.Update(time.Second)
	assert.Equal(t, second.volume, 0.75)

	m.FadeOut()
	m.Update(time.Second)
	assert.Equal(t, second.volume, 0.375)
	assert.Equal(t, second.stops, 0)
	m.Update(time.Second)
	assert.Equal(t, second.volume, 0.0)
	assert.Equal(t, second.stops, 1)
	assert.Equal(t, len(m.voices), 0)
}

// A third zone must not reset an already-outgoing track's fade. Returning to
// a still-fading track must reuse it from its live gain, without a volume jump.
func TestInterruptedMusicFades(t *testing.T) {
	t.Parallel()

	first := &recordingMusicStream{volume: 0.75}
	a := &musicVoice{
		label: "A", loop: true, player: first,
		volume: 0.75, start: 0.75, target: 0.75, elapsed: musicFadeDuration,
	}
	second := &recordingMusicStream{}
	b := &musicVoice{label: "B", loop: true, player: second}
	m := &MusicManager{cur: a, voices: []*musicVoice{a, b}}
	assert.NilError(t, m.PlayMusic("B", true, 0.25))
	m.Update(500 * time.Millisecond)
	assert.Equal(t, second.volume, 0.0390625, "the ramp must ease in rather than jump to its target")

	third := &recordingMusicStream{}
	c := &musicVoice{label: "C", loop: true, player: third}
	m.voices = append(m.voices, c)
	assert.NilError(t, m.PlayMusic("C", true, 0.5))
	m.Update(500 * time.Millisecond)

	before := second.volume

	assert.NilError(t, m.PlayMusic("B", true, 0.25))
	assert.Equal(t, len(m.voices), 3, "reuse the fading stream instead of opening a duplicate")
	assert.Equal(t, second.volume, before)
	assert.Equal(t, second.stops, 0)
	m.Update(time.Second)
	assert.Equal(t, first.stops, 1, "the first fade must finish two seconds after it began")
	assert.Equal(t, len(m.voices), 2)
	assert.Assert(t, second.volume > before && second.volume < 0.25)
	assert.Equal(t, m.cur, b)

	m.Stop()
	assert.Equal(t, first.stops, 1)
	assert.Equal(t, second.stops, 1)
	assert.Equal(t, third.stops, 1)
	assert.Equal(t, len(m.voices), 0)
	assert.Assert(t, m.cur == nil)
}
