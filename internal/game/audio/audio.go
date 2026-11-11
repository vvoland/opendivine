// SPDX-License-Identifier: GPL-3.0-only

// Package audio is OpenDivine's audio asset manager: it owns the
// process-wide ebiten audio context and exposes a MusicManager that
// resolves track labels to OGG payloads inside dat\sound.cmp.
//
// See re_docs/sound-runtime.md and re_docs/formats/sound.md for the
// engine-side details.
package audio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"

	"grono.dev/opendivine/pkg/assets/musicdat"
	"grono.dev/opendivine/pkg/assets/soundcmp"
)

// SampleRate is the ebiten audio-context mix rate; matches the OGG
// streams shipped in dat\sound.cmp (44100 Hz).
const SampleRate = 44100

const musicFadeDuration = 2 * time.Second

type musicStream interface {
	Play()
	SetVolume(float64)
	PauseAndStopReading()
}

type musicVoice struct {
	label  string
	loop   bool
	player musicStream

	volume  float64
	start   float64
	target  float64
	elapsed time.Duration
}

func (v *musicVoice) fadeTo(volume float64) {
	if v.target == volume {
		return
	}

	v.start, v.target, v.elapsed = v.volume, volume, 0
}

func (v *musicVoice) advance(dt time.Duration) {
	v.elapsed = min(v.elapsed+dt, musicFadeDuration)
	t := float64(v.elapsed) / float64(musicFadeDuration)
	// Smoothstep avoids abrupt changes in slope at either end of the fade.
	gain := t * t * (3 - 2*t)

	v.volume = v.start + (v.target-v.start)*gain
	if v.elapsed == musicFadeDuration {
		v.volume = v.target
	}

	v.player.SetVolume(v.volume)
}

var (
	ctxOnce sync.Once
	ctx     *audio.Context
)

// Context returns the process-wide ebiten audio context, creating it
// on first call.
func Context() *audio.Context {
	ctxOnce.Do(func() { ctx = audio.NewContext(SampleRate) })
	return ctx
}

// MusicManager owns the music.dat index, the sound.cmp archive
// handle, and the active and fading music streams. Build one with
// NewMusicManager and call Close when done to release the sound.cmp
// file handle.
type MusicManager struct {
	tracks   []musicdat.Track
	ambients []musicdat.Track
	zones    []musicdat.Zone
	byLabel  map[string]musicdat.Track

	arcFile *os.File
	arc     *soundcmp.Archive

	mu     sync.Mutex
	cur    *musicVoice
	voices []*musicVoice
}

// NewMusicManager parses sound\music.dat and mounts dat\sound.cmp
// under gamedataDir. The returned manager keeps the sound.cmp file
// open for its lifetime; callers should hold onto it for as long as
// they want to play music.
func NewMusicManager(gamedataDir string) (*MusicManager, error) {
	return newMusicManager(gamedataDir)
}

func newMusicManager(gamedataDir string) (*MusicManager, error) {
	mf, err := os.Open(filepath.Join(gamedataDir, "sound", "music.dat"))
	if err != nil {
		return nil, fmt.Errorf("open music.dat: %w", err)
	}
	defer mf.Close()
	md, err := musicdat.Decode(mf)
	if err != nil {
		return nil, fmt.Errorf("decode music.dat: %w", err)
	}

	arcFile, err := os.Open(filepath.Join(gamedataDir, "dat", "sound.cmp"))
	if err != nil {
		return nil, fmt.Errorf("open sound.cmp: %w", err)
	}
	arc, err := soundcmp.Open(arcFile)
	if err != nil {
		arcFile.Close()
		return nil, fmt.Errorf("read sound.cmp index: %w", err)
	}

	m := &MusicManager{
		tracks:   md.Tracks,
		ambients: md.Ambients,
		zones:    md.Zones,
		byLabel:  make(map[string]musicdat.Track, len(md.Tracks)+len(md.Ambients)),
		arcFile:  arcFile,
		arc:      arc,
	}
	// Tracks win on duplicate labels (no collisions in shipped data).
	for _, t := range md.Ambients {
		m.byLabel[t.Label] = t
	}
	for _, t := range md.Tracks {
		m.byLabel[t.Label] = t
	}
	return m, nil
}

// ErrUnknownTrack is returned by PlayMusic when label is not in
// music.dat.
var ErrUnknownTrack = errors.New("audio: unknown music track")

// PlayMusic crossfades to the named track at volume in [0, 1].
// A still-fading stream with the same label and loop mode is reused.
// If loop is true the stream restarts on completion.
// Label must be one of the music.dat labels (tracks and ambients share a
// flat namespace).
func (m *MusicManager) PlayMusic(label string, loop bool, volume float64) error {
	if math.IsNaN(volume) || volume < 0 || volume > 1 {
		return fmt.Errorf("audio: invalid music volume %v", volume)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, voice := range m.voices {
		if voice.label == label && voice.loop == loop {
			m.selectVoice(voice, volume)
			return nil
		}
	}

	t, ok := m.byLabel[label]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTrack, label)
	}
	ogg, err := m.arc.Read("wav\\music\\" + t.Filename)
	if err != nil {
		return fmt.Errorf("sound.cmp lookup %q: %w", t.Filename, err)
	}
	stream, err := vorbis.DecodeF32(bytes.NewReader(ogg))
	if err != nil {
		return fmt.Errorf("vorbis decode %q: %w", t.Filename, err)
	}

	var src io.Reader = stream
	if loop {
		src = audio.NewInfiniteLoopF32(stream, stream.Length())
	}
	pl, err := Context().NewPlayerF32(src)
	if err != nil {
		return fmt.Errorf("audio player: %w", err)
	}

	pl.SetVolume(0)
	voice := &musicVoice{label: label, loop: loop, player: pl}
	m.voices = append(m.voices, voice)
	m.selectVoice(voice, volume)
	pl.Play()
	return nil
}

func (m *MusicManager) selectVoice(voice *musicVoice, volume float64) {
	for _, other := range m.voices {
		if other != voice {
			other.fadeTo(0)
		}
	}

	m.cur = voice
	voice.fadeTo(volume)
}

// Update advances music fades by simulation time.
// Call it once per tick, including while the active zone stays unchanged.
func (m *MusicManager) Update(dt time.Duration) {
	if dt <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.voices = slices.DeleteFunc(m.voices, func(voice *musicVoice) bool {
		voice.advance(dt)

		if voice != m.cur && voice.target == 0 && voice.volume == 0 {
			voice.player.PauseAndStopReading()
			return true
		}

		return false
	})
}

// SetVolume fades the current music to a new volume without restarting it.
// Volume must be in [0, 1].
// With no current stream, it does nothing.
func (m *MusicManager) SetVolume(volume float64) error {
	if math.IsNaN(volume) || volume < 0 || volume > 1 {
		return fmt.Errorf("audio: invalid music volume %v", volume)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cur != nil {
		m.cur.fadeTo(volume)
	}

	return nil
}

// FadeOut fades all music to silence, then stops reading its streams.
func (m *MusicManager) FadeOut() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cur = nil
	for _, voice := range m.voices {
		voice.fadeTo(0)
	}
}

// Stop immediately halts all current and fading music streams.
// It is safe to call when nothing is playing.
func (m *MusicManager) Stop() {
	m.mu.Lock()
	voices := m.voices
	m.cur = nil
	m.voices = nil
	m.mu.Unlock()

	for _, voice := range voices {
		voice.player.PauseAndStopReading()
	}
}

// Close stops any playing music and releases the sound.cmp file
// handle. Further calls on the manager are undefined.
func (m *MusicManager) Close() error {
	m.Stop()
	if m.arcFile != nil {
		err := m.arcFile.Close()
		m.arcFile = nil
		return err
	}
	return nil
}

// Tracks returns music.dat section-1 entries.
func (m *MusicManager) Tracks() []musicdat.Track { return m.tracks }

// Ambients returns music.dat section-2 entries.
func (m *MusicManager) Ambients() []musicdat.Track { return m.ambients }
