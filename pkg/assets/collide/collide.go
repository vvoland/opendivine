// SPDX-License-Identifier: GPL-3.0-only

// Package collide reads static\imagelists\Collide.<n>: per-sprite
// 16-byte collision/cube records paired with the CPacked imagelists.
package collide

import (
	"encoding/binary"
	"fmt"
	"io"
)

// RecordSize is the on-disk size of one collide record (= 16 bytes).
const RecordSize = 16

// Record is one 16-byte cube — the per-sprite cube/anchor entry that
// pairs with CPacked sprite N at the same index. Fields confirmed
// against div.exe (CUBE/Cube.cpp).
//
// AnchorX / AnchorY are the sprite-local pixel offset from the
// sprite's top-left to the object's "ground point" (where the cube
// rests on the floor).  The engine reads these in
// FUN_00572100/0x00572100 and uses them to map world (X, Y) to the
// sprite's draw position: top-left = (worldX - AnchorX, worldY - AnchorY).
type Record struct {
	AnchorX int16 // [0] sprite-relative X of the cube anchor / "foot"
	AnchorY int16 // [1] sprite-relative Y of the cube anchor / "foot"
	RtTimer int16 // [2] runtime-only (always 0 in file)
	XExtent int16 // [3] right-edge offset from anchor for AI centre-X
	ZHeight int16 // [4] vertical Z extent of the cube (above ground)
	Width   int16 // [5] cube collision width
	Type    int16 // [6] 0=no collision, 1=static obstacle, 2=interactive
	Flags   int16 // [7] runtime flags (always 0 in file)
}

// File is the parsed collision table.
type File struct {
	Records []Record
}

// Decode reads a Collide.<n> stream.
func Decode(r io.Reader) (*File, error) {
	all, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("collide: read: %w", err)
	}
	if len(all)%RecordSize != 0 {
		return nil, fmt.Errorf("collide: size %d is not a multiple of %d", len(all), RecordSize)
	}
	out := &File{Records: make([]Record, len(all)/RecordSize)}
	for i := range out.Records {
		off := i * RecordSize
		r := &out.Records[i]
		r.AnchorX = int16(binary.LittleEndian.Uint16(all[off:]))
		r.AnchorY = int16(binary.LittleEndian.Uint16(all[off+2:]))
		r.RtTimer = int16(binary.LittleEndian.Uint16(all[off+4:]))
		r.XExtent = int16(binary.LittleEndian.Uint16(all[off+6:]))
		r.ZHeight = int16(binary.LittleEndian.Uint16(all[off+8:]))
		r.Width = int16(binary.LittleEndian.Uint16(all[off+10:]))
		r.Type = int16(binary.LittleEndian.Uint16(all[off+12:]))
		r.Flags = int16(binary.LittleEndian.Uint16(all[off+14:]))
	}
	return out, nil
}
