// SPDX-License-Identifier: GPL-3.0-only

// Package cpacked reads Divine Divinity image-list pairs:
//
//	CPackedb.<n>c (compressed blob)
//	CPackedi.<n>c (56-byte-stride index).
//
// Each blob entry is a u32 uncompressed-size header followed by an LZO1X-1 stream.
// This package returns the index entries and the post-LZO raw-cell payload bytes.
// The internal layout of the post-LZO cell buffer is not yet fully reversed.
package cpacked

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	lzo "github.com/anchore/go-lzo"
)

// EntrySize is the on-disk stride of one index record in CPackedi.
const EntrySize = 56

// FlagBits split the post-decode handling path the engine takes.
const (
	FlagStandard uint32 = 0x01
	FlagPalette  uint32 = 0x04
	FlagSpecial  uint32 = 0x10
)

// Entry is one row of the index file. Field names follow what the
// engine appears to use. Semantics are *partly* validated.
// WidthInner / HeightInner are typically Width-1 / Height-1 (so likely an
// inclusive max-coord), but a small fraction of entries deviate, suggesting
// they also encode anchor adjustments.
// PackedDims usually mirrors the same dims again ((Height << 16) | Width).
type Entry struct {
	BlobOffset  uint32
	Width       uint32
	Height      uint32
	Flags       uint32
	AnchorX     uint32
	AnchorY     uint32
	WidthInner  uint32
	HeightInner uint32
	PackedDims  uint32
	Reserved    [5]uint32
}

// Reader gives random access to entries and decompressed cell payloads.
type Reader struct {
	entries []Entry
	blob    io.ReaderAt
	blobLen int64
}

var (
	// ErrUnsupportedFlags is returned by DecodeCell when the entry's flags
	// select a path other than the standard sprite (e.g. raw 64x64,
	// palette, or special).
	ErrUnsupportedFlags = errors.New("cpacked: unsupported cell flags")

	ErrIndexAlignment  = errors.New("cpacked: index size is not a multiple of 56")
	ErrEntryOutOfRange = errors.New("cpacked: entry index out of range")
	ErrBadOffset       = errors.New("cpacked: blob offsets out of order or beyond blob length")
)

// NewReader parses the index file and binds it to a blob source.
// blobLen is required for last-entry size and bounds checks.
func NewReader(idx []byte, blob io.ReaderAt, blobLen int64) (*Reader, error) {
	if len(idx)%EntrySize != 0 {
		return nil, fmt.Errorf("%w: %d bytes", ErrIndexAlignment, len(idx))
	}
	n := len(idx) / EntrySize
	entries := make([]Entry, n)
	for i := range n {
		o := i * EntrySize
		entries[i] = Entry{
			BlobOffset:  binary.LittleEndian.Uint32(idx[o:]),
			Width:       binary.LittleEndian.Uint32(idx[o+4:]),
			Height:      binary.LittleEndian.Uint32(idx[o+8:]),
			Flags:       binary.LittleEndian.Uint32(idx[o+12:]),
			AnchorX:     binary.LittleEndian.Uint32(idx[o+16:]),
			AnchorY:     binary.LittleEndian.Uint32(idx[o+20:]),
			WidthInner:  binary.LittleEndian.Uint32(idx[o+24:]),
			HeightInner: binary.LittleEndian.Uint32(idx[o+28:]),
			PackedDims:  binary.LittleEndian.Uint32(idx[o+32:]),
		}
		for j := range 5 {
			entries[i].Reserved[j] = binary.LittleEndian.Uint32(idx[o+36+4*j:])
		}
	}
	return &Reader{entries: entries, blob: blob, blobLen: blobLen}, nil
}

// Count returns the number of entries.
func (r *Reader) Count() int { return len(r.entries) }

// Entry returns a copy of entry i.
func (r *Reader) Entry(i int) (Entry, error) {
	if i < 0 || i >= len(r.entries) {
		return Entry{}, fmt.Errorf("%w: %d (have %d)", ErrEntryOutOfRange, i, len(r.entries))
	}
	return r.entries[i], nil
}

// blobBounds returns [start, end) of the i-th compressed entry within the blob.
func (r *Reader) blobBounds(i int) (int64, int64, error) {
	start := int64(r.entries[i].BlobOffset)
	var end int64
	if i+1 < len(r.entries) {
		end = int64(r.entries[i+1].BlobOffset)
	} else {
		end = r.blobLen
	}
	if start < 0 || end < start || end > r.blobLen {
		return 0, 0, fmt.Errorf("%w: entry %d [%d, %d) blob=%d", ErrBadOffset, i, start, end, r.blobLen)
	}
	return start, end, nil
}

// CompressedPayload returns the LZO1X-1 stream for entry i
// (without the leading u32 uncompressed-size header).
func (r *Reader) CompressedPayload(i int) ([]byte, uint32, error) {
	if _, err := r.Entry(i); err != nil {
		return nil, 0, err
	}
	start, end, err := r.blobBounds(i)
	if err != nil {
		return nil, 0, err
	}
	if end-start < 4 {
		return nil, 0, fmt.Errorf("cpacked: entry %d truncated (size %d)", i, end-start)
	}
	var sz [4]byte
	if _, err := r.blob.ReadAt(sz[:], start); err != nil {
		return nil, 0, fmt.Errorf("cpacked: read uncomp_size for entry %d: %w", i, err)
	}
	uncomp := binary.LittleEndian.Uint32(sz[:])
	body := make([]byte, end-start-4)
	if _, err := r.blob.ReadAt(body, start+4); err != nil {
		return nil, 0, fmt.Errorf("cpacked: read body for entry %d: %w", i, err)
	}
	return body, uncomp, nil
}

// CellPayload decompresses entry i and returns its raw post-LZO bytes.
// The buffer is the engine's cell layout: header, per-line span table,
// and packed RGB565 pixel runs.
func (r *Reader) CellPayload(i int) ([]byte, error) {
	body, uncomp, err := r.CompressedPayload(i)
	if err != nil {
		return nil, err
	}
	dst := make([]byte, uncomp)
	n, err := lzo.Decompress(body, dst)
	if err != nil {
		return nil, fmt.Errorf("cpacked: lzo decompress entry %d: %w", i, err)
	}
	if uint32(n) != uncomp {
		return nil, fmt.Errorf("cpacked: entry %d decompressed to %d bytes, want %d", i, n, uncomp)
	}
	return dst, nil
}

// Cell is a decoded standard sprite cell — the result of parsing a
// flags=0x01 cell payload according to the per-line span table.
//
// Pixels are stored as RGB565 (2 bytes/pixel). The returned slice is
// row-major, length Width * Height * 2; pixels not covered by any
// span are zero-filled (transparent).
type Cell struct {
	Width    int
	Height   int
	AnchorX  int // origin offset within the cell, from the index entry
	AnchorY  int
	HotspotX int // sub_width  from packed_dims: x offset of the sprite's ground point from top-left
	HotspotY int // sub_height from packed_dims: y offset of the sprite's ground point from top-left
	RGB565   []byte
}

// DecodeCell parses entry i as a flags=0x01 sprite, returning the
// reconstructed RGB565 raster. Returns ErrUnsupportedFlags if the cell
// uses one of the non-standard paths (raw 64x64, palette, special).
func (r *Reader) DecodeCell(i int) (*Cell, error) {
	e, err := r.Entry(i)
	if err != nil {
		return nil, err
	}
	if e.Flags&FlagSpecial != 0 {
		return nil, fmt.Errorf("%w: entry %d flags=0x%x (special)", ErrUnsupportedFlags, i, e.Flags)
	}
	if e.Flags&FlagStandard == 0 {
		return nil, fmt.Errorf("%w: entry %d flags=0x%x (raw 64x64)", ErrUnsupportedFlags, i, e.Flags)
	}
	if e.Flags&FlagPalette != 0 {
		return nil, fmt.Errorf("%w: entry %d flags=0x%x (palette)", ErrUnsupportedFlags, i, e.Flags)
	}

	payload, err := r.CellPayload(i)
	if err != nil {
		return nil, err
	}
	if len(payload) < 12 {
		return nil, fmt.Errorf("cpacked: entry %d payload too short (%d bytes)", i, len(payload))
	}

	baseOff := binary.LittleEndian.Uint32(payload[4:8])
	numLines := int(binary.LittleEndian.Uint16(payload[10:12]))
	if numLines != int(e.Height) {
		return nil, fmt.Errorf("cpacked: entry %d num_lines (%d) != height (%d)", i, numLines, e.Height)
	}
	if int(baseOff) > len(payload) {
		return nil, fmt.Errorf("cpacked: entry %d base_offset (%d) past payload (%d)", i, baseOff, len(payload))
	}

	W, H := int(e.Width), int(e.Height)
	out := make([]byte, W*H*2)

	cursor := 12
	for y := range numLines {
		if cursor+4 > len(payload) {
			return nil, fmt.Errorf("cpacked: entry %d truncated at line %d", i, y)
		}
		numSpans := int(binary.LittleEndian.Uint16(payload[cursor : cursor+2]))
		if numSpans == 0 {
			cursor += 8 // empty line stride
			continue
		}
		// pixel_offset is misaligned — at cursor+2 — per the engine's
		// post-decode fixup at div.exe FUN_00558290 (writes back to
		// *(int *)((int)piVar2 + 2)).
		pixelOff := binary.LittleEndian.Uint32(payload[cursor+2 : cursor+6])
		spansStart := cursor + 6
		px := int(baseOff) + int(pixelOff)
		for s := range numSpans {
			off := spansStart + s*4
			if off+4 > len(payload) {
				return nil, fmt.Errorf("cpacked: entry %d span %d/%d truncated at line %d", i, s, numSpans, y)
			}
			startX := int(binary.LittleEndian.Uint16(payload[off : off+2]))
			length := int(binary.LittleEndian.Uint16(payload[off+2 : off+4]))
			if startX+length > W {
				return nil, fmt.Errorf("cpacked: entry %d span %d at line %d overflows width (start=%d len=%d w=%d)",
					i, s, y, startX, length, W)
			}
			if px+length*2 > len(payload) {
				return nil, fmt.Errorf("cpacked: entry %d pixel data overflow at line %d span %d", i, y, s)
			}
			rowOff := (y*W + startX) * 2
			copy(out[rowOff:rowOff+length*2], payload[px:px+length*2])
			px += length * 2
		}
		// Total advance per non-empty line is (numSpans + 2)*4 bytes.
		cursor += (numSpans + 2) * 4
	}

	return &Cell{
		Width:    W,
		Height:   H,
		AnchorX:  int(e.AnchorX),
		AnchorY:  int(e.AnchorY),
		HotspotX: int(e.PackedDims & 0xffff),
		HotspotY: int(e.PackedDims >> 16),
		RGB565:   out,
	}, nil
}
