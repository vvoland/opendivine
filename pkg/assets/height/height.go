// SPDX-License-Identifier: GPL-3.0-only

// Package height reads height.x<n>: the per-cell height/flags stream of
// the worldmap cell grid, shipped at main\startup\static\height.x<n> and
// copied to dynamic\ once a save is live. One 3-byte record per cell of
// the 0x200800-cell grid, in the grid's own v<<10 + u order:
//
//	u8  height    terrain height, in the units cell byte +7 holds
//	u16 flags     the saved cell-flag word
//
// The loader keeps one bit of that word: a set 0x200 hard-blocks the cell with
// a refcount of its own, and the climb bit 0x400 is DERIVED from the height
// byte rather than restored (re_docs/cell-grid.md, re_docs/render-trace.md).
// Both derivations belong to the grid that consumes them
// (internal/game/collision).
package height

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Grid geometry: the engine's own valid-index bound (`index <= 0x200800`
// gates every worldmap grid access), and the file carries exactly that
// many records.
const (
	Cols        = 1024        // cells per row; the grid index is v<<10 + u
	Rows        = 2050        // 0x200800 / 1024
	Cells       = Cols * Rows // 0x200800 = 2099200 cells
	RecordBytes = 3           // u8 height + u16 flags
	FileBytes   = Cells * RecordBytes
)

// FlagBlocker is the one saved flag bit the loader reads (bit 9): it
// hard-blocks the cell with `flags |= 0x201` and a refcount bump.
const FlagBlocker uint16 = 0x0200

// Map is a decoded height.x<n>: one height byte per grid cell plus the
// one saved bit the loader keeps.
type Map struct {
	// Heights is the per-cell terrain height, indexed by the engine's grid
	// index (v<<10 + u). Always Cells long, in the cell record's own unit:
	// the renderer multiplies it by 4 for world pixels (re_docs/render-trace.md).
	Heights []uint8

	// blocker is FlagBlocker packed one bit per cell — the loader keeps a
	// single bit, so this stays a bitset rather than a second 2 MB slice.
	blocker []uint64
}

// Decode reads a height.x<n> stream. The grid is fixed-size, so the
// buffer is sized by [FileBytes] and never by a count taken from the
// data; a file shorter or longer than the grid is rejected.
func Decode(r io.Reader) (*Map, error) {
	buf := make([]byte, FileBytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("height: read grid: %w", err)
	}
	var tail [1]byte
	switch _, err := io.ReadFull(r, tail[:]); err {
	case io.EOF: // exactly one grid, as every shipped file is
	case nil:
		return nil, fmt.Errorf("height: trailing bytes after the %d-byte grid", FileBytes)
	default:
		return nil, fmt.Errorf("height: check end of grid: %w", err)
	}

	m := &Map{
		Heights: make([]uint8, Cells),
		blocker: make([]uint64, (Cells+63)/64),
	}
	for i := range Cells {
		rec := buf[i*RecordBytes:]
		m.Heights[i] = rec[0]
		if binary.LittleEndian.Uint16(rec[1:3])&FlagBlocker != 0 {
			m.blocker[i/64] |= 1 << (uint(i) % 64)
		}
	}
	return m, nil
}

// Index returns the grid index of cell (u, v) — the engine's v<<10 + u
// with no bounds check, matching every call site (u > 1023 aliases into
// the following rows, as in the original).
func Index(u, v int) int { return v<<10 + u }

// Height returns the height of grid cell i, or 0 outside the grid.
func (m *Map) Height(i int) uint8 {
	if i < 0 || i >= len(m.Heights) {
		return 0
	}
	return m.Heights[i]
}

// Blocker reports whether grid cell i carries the kept [FlagBlocker] bit.
func (m *Map) Blocker(i int) bool {
	if i < 0 || i >= Cells {
		return false
	}
	return m.blocker[i/64]&(1<<(uint(i)%64)) != 0
}
