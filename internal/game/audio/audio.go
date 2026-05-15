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
	"os"
	"path/filepath"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"

	"grono.dev/opendivine/pkg/assets/musicdat"
	"grono.dev/opendivine/pkg/assets/soundcmp"
)

// SampleRate is the ebiten audio-context mix rate; matches the OGG
// streams shipped in dat\sound.cmp (44100 Hz).
const SampleRate = 44100

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
// handle, and the currently-playing music stream. Build one with
// NewMusicManager and call Close when done to release the sound.cmp
// file handle.
type MusicManager struct {
	tracks   []musicdat.Track
	ambients []musicdat.Track
	byLabel  map[string]musicdat.Track

	arcFile *os.File
	arc     *soundcmp.Archive

	mu  sync.Mutex
	cur *audio.Player
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

// PlayMusic stops any currently playing music and starts the named
// track. If loop is true the stream restarts on completion. label
// must be one of the music.dat labels (tracks and ambients share a
// flat namespace).
func (m *MusicManager) PlayMusic(label string, loop bool) error {
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

	m.mu.Lock()
	old := m.cur
	m.cur = pl
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	pl.Play()
	return nil
}

// Stop halts the currently playing music. Safe to call when nothing
// is playing.
func (m *MusicManager) Stop() {
	m.mu.Lock()
	cur := m.cur
	m.cur = nil
	m.mu.Unlock()
	if cur != nil {
		_ = cur.Close()
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
