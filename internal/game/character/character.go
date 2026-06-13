// SPDX-License-Identifier: GPL-3.0-only

package character

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"grono.dev/opendivine/pkg/assets/agentclass"
	"grono.dev/opendivine/pkg/assets/heroes"
)

// Character is a renderable + animated entity built from one or more hero-bank
// layers.
// Used for the player and for any NPC that draws from static\heroes\*.
type Character struct {
	X, Y     float64 // world position
	Dir      int     // 0..7 iso facing, clockwise from E (0=E, 2=S, 4=W, 6=N)
	Moving   bool
	AnimTick int
	AnimIdx  int
	AnimSlot int                // engine anim slot (default 11 = 'G' = stand idle)
	Equip    CharacterEquipment // currently-equipped items
	Layers   []heroLayer

	// dirCounts: per-anim-slot direction count loaded from
	// data.000's AgentClasses section ("Hero" class)
	// Indexed by AnimSlot (0..18); 0 = slot disabled.
	// See pkg/assets/agentclass.
	dirCounts [19]uint8

	// PinAnim: when true, Step() does not advance/reset AnimSlot,
	// AnimIdx, or AnimTick, used by the -walk and -dir debug
	// flags so a forced animation state stays put across frames.
	PinAnim bool

	// ForceSlot: when >= 0, override AnimSlot to this value while still letting
	// Step advance AnimIdx so the cycle plays.
	ForceSlot int

	// ClassAnchorX, ClassAnchorY: the composite-frame point that maps to
	// the character's world (X, Y) standing position.  X is the frame
	// centre (MaxWidth/2); Y is the per-class foot line (see heroFootY).
	ClassAnchorX, ClassAnchorY int

	// CompMaxHeight: per-class .key "Max size" height, full vertical
	// extent of the composite-frame bbox.
	// Used by CameraTarget to place the screen centre on the character's
	// mid-body, not on layer A's hotspot (which sits ~60% of the way down the
	// figure).
	CompMaxHeight int
}

// CameraTarget returns the world position the camera should centre on to keep
// the whole character visible.
// The character extends from world Y − ClassAnchorY (top of the composite bbox)
// down to world Y + (CompMaxHeight − ClassAnchorY) (bottom).
//
// Centring on the bbox midpoint instead of on layer A's hotspot avoids clipping
// the head at high zooms.
func (c *Character) CameraTarget() (float64, float64) {
	bias := float64(c.ClassAnchorY) - float64(c.CompMaxHeight)/2
	return c.X, c.Y - bias
}

// heroFootY is the per-class foot line: the composite-frame Y that the
// character's world Y maps onto.  Verified against the decoded legs; see
// re_docs/render-hero.md.
var heroFootY = map[string]int{
	"surm": 158,
	"surf": 146,
	"warm": 154,
	"warf": 150,
	"wizm": 192,
	"wizf": 184,
}

// heroLayer is one body/equipment layer in a character's composite stack.
// Each variant of a class (.bic file) lives as one layer.
// Frames are decoded on demand and cached as ebiten.Images.
type heroLayer struct {
	variant string // "A".."E"
	key     *heroes.Key
	idc     []heroes.IDCRecord
	bic     *heroes.BIC
	frames  map[int]*ebiten.Image
	byName  map[string]int // .key group name → index into key.Groups
}

// Load opens one or more hero-bank layers for a class.
// The shared <class>.key file lists groups for ALL appearance variants (the 3rd
// char of every group name is the variant letter), so we filter per layer
// before constructing the BIC reader.
func Load(gameDir, class string, variants ...string) (*Character, error) {
	dir := gameDir + "/static/heroes"
	keyData, err := os.ReadFile(fmt.Sprintf("%s/%s.key", dir, class))
	if err != nil {
		return nil, fmt.Errorf("character %s key: %w", class, err)
	}
	k, err := heroes.DecodeKey(bytes.NewReader(keyData))
	if err != nil {
		return nil, fmt.Errorf("character %s decode key: %w", class, err)
	}
	c := &Character{Dir: 2, ForceSlot: -1} // default south-facing
	// Load per-anim-slot direction counts from data.000's AgentClasses section.
	if dataF, err := os.Open(gameDir + "/main/startup/data.000"); err == nil {
		if hero, err := agentclass.ReadHero(dataF); err == nil {
			c.dirCounts = hero.DirCounts
		}
		dataF.Close()
	}
	// World (X, Y) is the standing point: horizontally the frame centre,
	// vertically the per-class foot line.
	c.ClassAnchorX = k.MaxWidth / 2
	if y, ok := heroFootY[strings.ToLower(class)]; ok {
		c.ClassAnchorY = y
	} else {
		c.ClassAnchorY = k.MaxHeight
	}
	c.CompMaxHeight = k.MaxHeight
	for _, variant := range variants {
		v := strings.ToLower(variant)
		filt := &heroes.Key{
			MaxWidth:  k.MaxWidth,
			MaxHeight: k.MaxHeight,
			CenterX:   k.CenterX,
			CenterY:   k.CenterY,
		}
		for _, gr := range k.Groups {
			if len(gr.Name) >= 3 && strings.ToLower(gr.Name[2:3]) == v {
				filt.Groups = append(filt.Groups, gr)
			}
		}
		idcData, err := os.ReadFile(fmt.Sprintf("%s/%s%s.idc", dir, class, variant))
		if err != nil {
			return nil, fmt.Errorf("character %s%s idc: %w", class, variant, err)
		}
		idc, err := heroes.DecodeIDC(bytes.NewReader(idcData))
		if err != nil {
			return nil, fmt.Errorf("character %s%s decode idc: %w", class, variant, err)
		}
		bicData, err := os.ReadFile(fmt.Sprintf("%s/%s%s.bic", dir, class, variant))
		if err != nil {
			return nil, fmt.Errorf("character %s%s bic: %w", class, variant, err)
		}
		bic, err := heroes.OpenBIC(bicData, filt, idc)
		if err != nil {
			return nil, fmt.Errorf("character %s%s open bic: %w", class, variant, err)
		}
		c.Layers = append(c.Layers, heroLayer{
			variant: variant,
			key:     filt,
			idc:     idc,
			bic:     bic,
			frames:  map[int]*ebiten.Image{},
		})
	}
	// Build per-variant group-name → key index map so the composer
	// (composeLayerFrames) can look up groups by name like "MAA0" .key files
	// are case-mixed (warm.key has lowercase "maa0", surm.key has "mya0"); the
	// engine uses _stricmp in FUN_0050ac30, so normalise to uppercase here and
	// on lookup.
	for li := range c.Layers {
		c.Layers[li].byName = map[string]int{}
		for gi, gr := range c.Layers[li].key.Groups {
			c.Layers[li].byName[strings.ToUpper(gr.Name)] = gi
		}
	}
	return c, nil
}

// Step advances the walk-cycle tick.
// Call once per Update with the movement delta (zero when idle).
// Updates Dir/Moving/Anim* state.
//
// When ForceSlot >= 0, AnimSlot is held to that value (debug) the tick still
// advances so the animation cycles.
func (c *Character) Step(dx, dy float64) {
	moving := dx != 0 || dy != 0
	if c.PinAnim {
		c.Moving = moving
		return
	}
	advance := func() {
		c.AnimTick++
		if c.AnimTick >= 6 {
			c.AnimTick = 0
			c.AnimIdx++
		}
	}
	if c.ForceSlot >= 0 {
		// Debug: hold AnimSlot, but keep cycling.  Reset on entry.
		if c.AnimSlot != c.ForceSlot {
			c.AnimTick = 0
			c.AnimIdx = 0
			c.AnimSlot = c.ForceSlot
		} else {
			advance()
		}
		// Update Dir if moving so debug walking faces correctly.
		if moving {
			ang := math.Atan2(dy, dx)
			oct := int(math.Round(ang/(math.Pi/4))) + 8
			c.Dir = oct % 8
		}
		c.Moving = moving
		return
	}
	if moving {
		ang := math.Atan2(dy, dx)
		oct := int(math.Round(ang/(math.Pi/4))) + 8
		c.Dir = oct % 8
		advance()
		c.AnimSlot = 1 // walk (cVar4='A')
	} else {
		// Idle slot depends on weapon: 0 ('B') unarmed,
		// 6 ('H' or helmet[0]) when wielding a weapon.
		//
		// The per-direction frame block (8 frames for B, varies per action) is
		// a real breathing animation, let it cycle while standing still.
		//
		// Reset AnimIdx only on transition into idle from another slot.
		// dirCount=40 keeps the cycle within one 9° direction band (wider
		// dirCount would make AnimIdx cross direction boundaries and produce
		// visible rotation).
		idleSlot := 0
		if c.Equip.Weapon != 0 {
			idleSlot = 6
		}
		if c.AnimSlot != idleSlot {
			c.AnimTick = 0
			c.AnimIdx = 0
			c.AnimSlot = idleSlot
		} else {
			advance()
		}
	}
	c.Moving = moving
}

// layerFrame picks the IDC frame id for the given layer index given the
// character's current animation slot, direction, and equipment.
// Returns -1 if the layer doesn't apply (composer skipped it, or the named
// group doesn't exist in this variant's filtered key).
//
// The composer (composeLayerNames) gives per-variant group names like "MAA0" /
// "MGB0" / "MGD0".
// Each layer looks up the group by name in its own key and emits the first
// frame of the requested direction sub-group.
func (c *Character) layerFrame(li int) int {
	if li >= len(c.Layers) {
		return -1
	}
	l := c.Layers[li]
	if l.key == nil {
		return -1
	}
	names := composeLayerNames(c.AnimSlot, c.Equip)
	// Variants A, B, C, D, E live in layers 0..4, find which slot
	// in the names array corresponds to this layer's variant.
	want := variantSlot(l.variant)
	if want < 0 || names[want] == "" {
		return -1
	}
	name := strings.ToUpper(names[want])
	gi, ok := l.byName[name]
	if !ok {
		// Group missing for this variant (e.g. wizf has no MGD groups for the
		// idle face).
		// Fall back to the same-action variant-A name with the requested action
		// letter rewritten to 'A' (walk), so layer D shows the walk-cycle face
		// during idle.
		// Empirically: classes that lack a per-action face group reuse MAD
		// frames for that action.
		if len(name) >= 2 {
			fallback := "MA" + name[2:]
			if gi2, ok2 := l.byName[fallback]; ok2 {
				gi = gi2
				ok = true
			}
		}
		if !ok {
			return -1
		}
	}
	grp := l.key.Groups[gi]
	// FUN_0050ac30 divides the group's total frame count by `param_7`
	// (directions) and treats each stride as one direction, directions are
	// concatenated within the single named group.
	// The 4th char of the group name is the equipment/variation index stamped
	// in by FUN_00439b70 (e.g. MGB0/MGB1/MGB2/MGB3 are different
	// torso-equipment variants of the G action, each carrying ALL directions).
	//
	// Per-anim-slot direction count comes from data.000's AgentClasses block
	// (see pkg/assets/agentclass).
	dirCount := 20
	if c.AnimSlot >= 0 && c.AnimSlot < len(c.dirCounts) && c.dirCounts[c.AnimSlot] > 0 {
		dirCount = int(c.dirCounts[c.AnimSlot])
	}
	framesPerDir := max((grp.End-grp.Start)/dirCount, 1)
	// our_Dir (CW from E) → engine_dir (CCW from N) for any N:
	//   cardinals at 3N/4, N/2, N/4, 0 (E/S/W/N)
	//   each our_Dir step = N/8 engine steps; multiply before
	//   dividing so the 1/8 fraction works for non-multiples of 8
	d := c.Dir % 8
	if d < 0 {
		d += 8
	}
	engineDir := (3*dirCount/4 - d*dirCount/8 + dirCount) % dirCount
	// Idle slots (0 = unarmed B, 6 = armed H) ping-pong through the
	// per-direction block, hard-reset reads as "cut off".
	frameInDir := pingPongOrLoop(c.AnimIdx, framesPerDir, c.AnimSlot == 0 || c.AnimSlot == 6)
	return grp.Start + engineDir*framesPerDir + frameInDir
}

func pingPongOrLoop(animIdx, n int, pingPong bool) int {
	if n <= 1 {
		return 0
	}
	if !pingPong {
		f := animIdx % n
		if f < 0 {
			f += n
		}
		return f
	}
	period := 2 * (n - 1)
	phase := animIdx % period
	if phase < 0 {
		phase += period
	}
	if phase >= n {
		return period - phase
	}
	return phase
}

func variantSlot(v string) int {
	if len(v) == 0 {
		return -1
	}
	switch v[0] {
	case 'A', 'a':
		return 0
	case 'B', 'b':
		return 1
	case 'C', 'c':
		return 2
	case 'D', 'd':
		return 3
	case 'E', 'e':
		return 4
	}
	return -1
}

func (c *Character) layerSprite(layer, frame int) (*ebiten.Image, heroes.IDCRecord) {
	var rec heroes.IDCRecord
	if layer >= len(c.Layers) {
		return nil, rec
	}
	l := c.Layers[layer]
	if l.bic == nil || frame < 0 || frame >= len(l.idc) {
		return nil, rec
	}
	rec = l.idc[frame]
	if img, ok := l.frames[frame]; ok {
		return img, rec
	}
	src, err := l.bic.Frame(rec, frame)
	if err != nil {
		log.Printf("character layer %d frame %d: %v", layer, frame, err)
		l.frames[frame] = nil
		return nil, rec
	}
	img := ebiten.NewImageFromImage(src)
	l.frames[frame] = img
	return img, rec
}

// Draw renders all of the character's layers at its current world
// position, applying mirroring per the iso direction map.
//
// Composition algorithm (engine-faithful, per FUN_0050ac30 +
// per-class constants from FUN_0050bb10):
//
// - Layer A is the anchor; its hotspot lands at (ClassAnchorX, ClassAnchorY)
// within the per-class composite frame.
// - Layers B/C/D/E each have per-frame attach pairs in their .idc record's tail
// (12 int16s = 6 (x,y) pairs). The pair each variant owns gives the position in
// composite-frame coordinates where THAT layer's hotspot lands.
// - World mapping: layer L's hotspot in world = (X, Y) + (layerAttach −
// classAnchor).
//
// Variant-to-pair-slot ownership (verified by examining surm/warm .idc data,
// variant A has all -1; B always populates pairs 0/1/2; C populates 4/5; D
// populates 3; E populates none, uses one of B's hand pairs):
//
//	A - none (uses class anchor)
//	B - pair 0
//	C - pair 4 (head-mount of helmet)
//	D - pair 3 (face/neck)
//	E - pair 1 (one of B's hand attach points)
//
// Layers come in (A, B, C, D, E) order and are drawn back-to-front.
//
// worldToScreen converts world pixels to ebiten screen pixels for
// the active camera + zoom.
// Returns true if anything was drawn.
func (c *Character) Draw(screen *ebiten.Image, zoom float64, worldToScreen func(wx, wy float64) (float64, float64)) bool {
	// Each layer's IDC record carries (XMin, YMin, Width, Height) in COMPOSITE
	// coordinates.
	// The agent's world (X, Y) maps to the per-class composite anchor
	// (ClassAnchorX, ClassAnchorY).
	// Therefore each layer's world top-left is:
	//   (X + XMin - ClassAnchorX, Y + YMin - ClassAnchorY)
	// and its bbox is Width × Height.  Engine ref: FUN_0050ac30
	drewAny := false
	for li := range c.Layers {
		fid := c.layerFrame(li)
		if fid < 0 {
			continue
		}
		img, rec := c.layerSprite(li, fid)
		if img == nil {
			continue
		}
		tlX := c.X + float64(int(rec.XMin)-c.ClassAnchorX)
		tlY := c.Y + float64(int(rec.YMin)-c.ClassAnchorY)
		sx, sy := worldToScreen(tlX, tlY)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(zoom, zoom)
		op.GeoM.Translate(sx, sy)
		screen.DrawImage(img, op)
		drewAny = true
	}
	return drewAny
}
