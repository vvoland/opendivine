// SPDX-License-Identifier: GPL-3.0-only

// Package objects reads Divine Divinity's static\objects.000: the
// global object catalogue (148-byte struct x 7208 entries, parallel
// to CPacked imagelist 0).
package objects

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// EntrySize is the on-disk stride.
const EntrySize = 0x94

// Static-behaviour bit indices in `Object.SBFlags`. Order taken from
// the engine's CSV exporter at div.exe:0x00581520.
const (
	SBSleep             = 1 << 0
	SBTransparent       = 1 << 1
	SBShadow            = 1 << 2
	SBUseClass          = 1 << 3
	SBRealBlack         = 1 << 4
	SBForceBackwall     = 1 << 5
	SBForceLeftwall     = 1 << 6
	SBForceFloor        = 1 << 7
	SBNoLookThrough     = 1 << 8
	SBTwinkle           = 1 << 9
	SBBow               = 1 << 10
	SBDirectMove        = 1 << 11
	SBAmbientSound      = 1 << 12
	SBLightBlocker      = 1 << 13
	SBLightBridge       = 1 << 14
	SBDontLoopAnimation = 1 << 15
	SBMakeFloating      = 1 << 16
	SBWalkOn            = 1 << 17
	SBAdditive          = 1 << 18
	SBNeedPerfectMatch  = 1 << 19
	SBShowInObjectBox   = 1 << 20
	SBPutOn             = 1 << 21
)

// Object is one parsed entry of objects.000.
type Object struct {
	// Header — pre-name fields.
	FlagsA         uint32   // +0x00 — s_* state bits
	SubValues      [16]byte // +0x04 — packed value pool referenced by FlagsA bits
	Weight         uint32   // +0x14
	AnimationIndex int32    // +0x18 — index into APacked anim sets; -1 = no anim
	SBFlags        uint32   // +0x1c — 22-bit static-behaviour bitfield (see SB* consts)

	// Name — 32-byte NUL-padded ASCII at +0x20.
	Name string

	// Post-name fields.
	ID                     uint32 // +0x30 — identical to file index, written by loader
	Class                  uint32 // +0x34
	BreakAnimationIndex    uint32 // +0x38
	ClothingCode           string // +0x3c — 16-byte string in struct, NUL-padded
	FloatingImageIndex     uint32 // +0x4c
	FloatingListIndex      uint32 // +0x50
	FloatingHighlightIndex uint32 // +0x54
	FloatingPressedIndex   uint32 // +0x58
	FloatingDisabledIndex  uint32 // +0x5c
	// Sprite-local pixel coordinates of the object's ground / cube
	// anchor.  The engine adds these to the object's world (X, Y) to
	// produce the spatial-hash bucket key (FUN_00582890) and draws the
	// sprite so this pixel sits at world (X, Y).  The 8 bytes at +0x60
	// were previously documented as runtime-only padding — they are
	// not: the +0x60..+0x63 pair is initialised to (-1, -1) for every
	// shipped entry, and +0x64/+0x66 carry the real anchor.
	AnchorX            int16  // +0x64
	AnchorY            int16  // +0x66
	WeaponAnimation    uint32 // +0x68
	TradePriority      uint32 // +0x6c
	FloatingGroup      uint32 // +0x70
	AutomapEntry       uint32 // +0x7c
	BridgePatchXOffset int16  // +0x80
	BridgePatchYOffset int16  // +0x82
	BridgePatchXSize   int16  // +0x84
	BridgePatchYSize   int16  // +0x86
}

// Catalog is the whole objects.000 file — a flat slice indexed by id.
type Catalog struct {
	Entries []Object
}

// Errors reported by this package.
var (
	ErrSizeNotMultiple = errors.New("objects: file size is not a multiple of 148")
	ErrShortRead       = errors.New("objects: short read")
)

// Decode parses an objects.000 file from r.
func Decode(r io.Reader) (*Catalog, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(buf)%EntrySize != 0 {
		return nil, fmt.Errorf("%w: %d bytes", ErrSizeNotMultiple, len(buf))
	}
	n := len(buf) / EntrySize
	out := &Catalog{Entries: make([]Object, n)}
	for i := range n {
		base := i * EntrySize
		o := &out.Entries[i]
		o.FlagsA = binary.LittleEndian.Uint32(buf[base+0x00:])
		copy(o.SubValues[:], buf[base+0x04:base+0x14])
		o.Weight = binary.LittleEndian.Uint32(buf[base+0x14:])
		o.AnimationIndex = int32(binary.LittleEndian.Uint32(buf[base+0x18:]))
		o.SBFlags = binary.LittleEndian.Uint32(buf[base+0x1c:])
		o.Name = readNulPadded(buf[base+0x20 : base+0x40])
		o.ID = binary.LittleEndian.Uint32(buf[base+0x30:])
		o.Class = binary.LittleEndian.Uint32(buf[base+0x34:])
		o.BreakAnimationIndex = binary.LittleEndian.Uint32(buf[base+0x38:])
		o.ClothingCode = readNulPadded(buf[base+0x3c : base+0x4c])
		o.FloatingImageIndex = binary.LittleEndian.Uint32(buf[base+0x4c:])
		o.FloatingListIndex = binary.LittleEndian.Uint32(buf[base+0x50:])
		o.FloatingHighlightIndex = binary.LittleEndian.Uint32(buf[base+0x54:])
		o.FloatingPressedIndex = binary.LittleEndian.Uint32(buf[base+0x58:])
		o.FloatingDisabledIndex = binary.LittleEndian.Uint32(buf[base+0x5c:])
		o.AnchorX = int16(binary.LittleEndian.Uint16(buf[base+0x64:]))
		o.AnchorY = int16(binary.LittleEndian.Uint16(buf[base+0x66:]))
		o.WeaponAnimation = binary.LittleEndian.Uint32(buf[base+0x68:])
		o.TradePriority = binary.LittleEndian.Uint32(buf[base+0x6c:])
		o.FloatingGroup = binary.LittleEndian.Uint32(buf[base+0x70:])
		o.AutomapEntry = binary.LittleEndian.Uint32(buf[base+0x7c:])
		o.BridgePatchXOffset = int16(binary.LittleEndian.Uint16(buf[base+0x80:]))
		o.BridgePatchYOffset = int16(binary.LittleEndian.Uint16(buf[base+0x82:]))
		o.BridgePatchXSize = int16(binary.LittleEndian.Uint16(buf[base+0x84:]))
		o.BridgePatchYSize = int16(binary.LittleEndian.Uint16(buf[base+0x86:]))
	}
	return out, nil
}

// HasSB returns whether the entry has a particular static-behaviour
// flag set. Use one of the SB* constants.
func (o Object) HasSB(mask uint32) bool { return o.SBFlags&mask != 0 }

func readNulPadded(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
