// SPDX-License-Identifier: GPL-3.0-only

package game

import (
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"grono.dev/opendivine/internal/game/audio"
	"grono.dev/opendivine/internal/game/mainmenu"
	"grono.dev/opendivine/pkg/assets/tga"
)

// Config is the input to New / Run. Every field is a plain value;
// callers (cmd/divine and any future GUI launcher) are responsible
// for picking sensible defaults before passing a Config in. The
// debug-only Dir / WalkFrame / Slot fields use -1 to mean "no
// override".
type Config struct {
	// GamedataDir is the resolved path to a Divine Divinity install.
	GamedataDir string

	// Screenshot, when non-empty, makes the game render exactly one frame, save
	// it to this path as PNG, and exit.
	Screenshot string

	// Region is the world partition to load (0..4). Pre-populate from
	// ReadSpawn().Region for the engine-faithful default.
	Region int

	// Zoom is the initial render zoom; 1.0 = native 1:1 pixel.
	Zoom float64

	// PosX, PosY are the starting world position in pixels. Pre- populate from
	// ReadSpawn() for the survivor's basement.
	PosX, PosY float64

	// WindowW, WindowH set the initial window size.
	WindowW, WindowH int

	// Class is the hero class folder name under static\heroes\.
	Class string

	// SkipMenu boots directly into the world (legacy path / dev shortcut).
	SkipMenu bool

	// MenuMusic, if non-empty, forces a specific music.dat label for
	// menu playback (e.g. "1", "forest"). Empty = first track.
	MenuMusic string

	// Debug overrides. -1 = no override.
	Dir       int // force player Dir 0..7
	WalkFrame int // force walk anim (slot 1) at this AnimIdx
	Slot      int // force AnimSlot
}

// Run boots OpenDivine. By default it opens the main menu; choosing
// "New Game" then constructs the in-world *Game and swaps it in.
// SkipMenu bypasses the menu and goes straight to the world (matches
// the pre-menu boot behaviour).
func Run(cfg Config) error {
	// The original engine advances simulation at 40 ticks per second. Keep
	// animation and movement rates independent of Ebiten's 60 TPS default.
	ebiten.SetTPS(engineTPS)
	if cfg.Screenshot != "" {
		g, err := New(cfg)
		if err != nil {
			return err
		}
		return ebiten.RunGame(g)
	}

	ebiten.SetWindowSize(cfg.WindowW, cfg.WindowH)
	ebiten.SetWindowTitle("OpenDivine")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	music, err := audio.NewMusicManager(cfg.GamedataDir)
	if err != nil {
		log.Printf("audio init: %v (continuing silently)", err)
	} else {
		defer music.Close()
	}

	if cfg.SkipMenu {
		g, err := New(cfg)
		if err != nil {
			return err
		}

		g.startRegionMusic(music)

		return ebiten.RunGame(&shell{current: gameScreen{g: g}, music: music})
	}

	menu := mainmenu.New(false /* no live game yet */)
	if back, err := loadMenuBackdrop(cfg.GamedataDir, cfg.WindowW); err != nil {
		log.Printf("menu backdrop: %v (using gradient fallback)", err)
	} else {
		menu.Backdrop = back
	}

	if music != nil {
		label := cfg.MenuMusic
		if label == "" {
			tracks := music.Tracks()
			if len(tracks) > 0 {
				label = "Title"
			}
		}
		if label != "" {
			if err := music.PlayMusic(label, true, 1); err != nil {
				log.Printf("menu music: %v", err)
			} else {
				log.Printf("menu music: playing track %q", label)
			}
		}
	}

	sh := &shell{current: &menuScreen{menu: menu, music: music, cfg: cfg}, music: music}

	return ebiten.RunGame(sh)
}

// menuScreen adapts mainmenu.Menu to Screen and handles transitions out of it.
// Run owns the music manager across menu and world playback.
type menuScreen struct {
	menu  *mainmenu.Menu
	music *audio.MusicManager // may be nil if audio init failed
	cfg   Config
}

func (s *menuScreen) Update() (Screen, error) {
	if err := s.menu.Update(); err != nil {
		return nil, err
	}
	switch s.menu.TakeAction() {
	case mainmenu.ActionQuit:
		return ScreenQuit, nil
	case mainmenu.ActionNewGame:
		g, err := New(s.cfg)
		if err != nil {
			log.Printf("new game: %v", err)
			return nil, nil
		}

		g.startRegionMusic(s.music)

		return gameScreen{g: g}, nil
	case mainmenu.ActionResume,
		mainmenu.ActionLoadGame,
		mainmenu.ActionSaveGame,
		mainmenu.ActionOptions,
		mainmenu.ActionViewIntro,
		mainmenu.ActionCredits:
		log.Printf("menu action not implemented yet")
	}
	return nil, nil
}

func (s *menuScreen) Draw(dst *ebiten.Image)     { s.menu.Draw(dst) }
func (s *menuScreen) Layout(w, h int) (int, int) { return s.menu.Layout(w, h) }

// loadMenuBackdrop picks the resolution-appropriate static\back*.tga
// (backs.tga at 640, backe.tga at 1024, back.tga otherwise) and
// decodes it into an *ebiten.Image.
func loadMenuBackdrop(gamedataDir string, winW int) (*ebiten.Image, error) {
	name := "back.tga"
	switch winW {
	case 640:
		name = "backs.tga"
	case 1024:
		name = "backe.tga"
	}
	path := filepath.Join(gamedataDir, "static", name)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := tga.Decode(f)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImage(img), nil
}
