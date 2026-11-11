// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"grono.dev/opendivine/internal/game/audio"
)

// Screen is the unit of high-level state in OpenDivine: main menu,
// in-world simulation, future load/save UI, etc. The shell forwards
// ebiten's Update/Draw/Layout to whichever screen is active.
//
// A Screen returns a non-nil *next* from Update to swap itself out;
// returning the special sentinel ScreenQuit terminates the program.
type Screen interface {
	Update() (next Screen, err error)
	Draw(*ebiten.Image)
	Layout(outW, outH int) (int, int)
}

// ScreenQuit, returned from a Screen.Update, asks the shell to exit
// via ebiten.Termination on the next tick.
var ScreenQuit Screen = quitSentinel{}

type quitSentinel struct{}

func (quitSentinel) Update() (Screen, error)    { return nil, nil }
func (quitSentinel) Draw(*ebiten.Image)         {}
func (quitSentinel) Layout(w, h int) (int, int) { return w, h }

// gameScreen wraps an in-world *Game (which already implements
// ebiten.Game's Update/Draw/Layout) to satisfy Screen. The world
// screen never voluntarily yields control, so Update always returns
// (nil, err).
type gameScreen struct{ g *Game }

func (s gameScreen) Update() (Screen, error) {
	return nil, s.g.Update()
}
func (s gameScreen) Draw(dst *ebiten.Image)     { s.g.Draw(dst) }
func (s gameScreen) Layout(w, h int) (int, int) { return s.g.Layout(w, h) }

// shell is the top-level ebiten.Game. It holds a single active Screen
// and forwards lifecycle methods to it, handling Screen swaps and the
// ScreenQuit sentinel.
type shell struct {
	current Screen
	music   *audio.MusicManager
}

func (s *shell) Update() error {
	next, err := s.current.Update()
	if err != nil {
		return err
	}
	if next == ScreenQuit {
		return ebiten.Termination
	}
	if next != nil {
		s.current = next
	}

	if s.music != nil {
		s.music.Update(time.Second / engineTPS)
	}

	return nil
}

func (s *shell) Draw(screen *ebiten.Image) { s.current.Draw(screen) }

func (s *shell) Layout(w, h int) (int, int) { return s.current.Layout(w, h) }
