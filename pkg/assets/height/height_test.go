// SPDX-License-Identifier: GPL-3.0-only

package height_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"grono.dev/opendivine/internal/testutils"
	"grono.dev/opendivine/pkg/assets/height"
)

// TestSizeIdentity pins the layout identity the grid geometry implies: one
// 3-byte record per cell of the 0x200800-cell worldmap grid, so every
// shipped height.x<n> is exactly 3 * 0x200800 = 6,297,600 bytes.
func TestSizeIdentity(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	for n := range 5 {
		name := fmt.Sprintf("height.x%d", n)
		t.Run(name, func(t *testing.T) {
			fi, err := os.Stat(filepath.Join(gamedata, "main/startup/static", name))
			assert.NilError(t, err)
			assert.Equal(t, fi.Size(), int64(height.FileBytes),
				"%s is not 3 bytes per grid cell", name)
		})
	}
}

// TestDecodeSynthetic covers the decoder: the record's field order, the one
// bit the loader keeps, the two size errors, and reads past either end.
func TestDecodeSynthetic(t *testing.T) {
	grid := func(cells map[int][3]byte) []byte {
		b := make([]byte, height.FileBytes)
		for i, rec := range cells {
			copy(b[i*height.RecordBytes:], rec[:])
		}
		return b
	}
	rec := func(h uint8, flags uint16) [3]byte {
		var r [3]byte
		r[0] = h
		binary.LittleEndian.PutUint16(r[1:], flags)
		return r
	}

	t.Run("fields and kept bit", func(t *testing.T) {
		// 0x1201 is a common shipped value: the kept 0x200, the blocker
		// bit 0x1 and a refcount nibble the loader rebuilds itself.
		m, err := height.Decode(bytes.NewReader(grid(map[int][3]byte{
			0:                  rec(17, 0x1201),
			1:                  rec(80, 0x0400),
			height.Cells - 1:   rec(255, 0xffff),
			height.Index(3, 2): rec(40, 0x0208),
		})))
		assert.NilError(t, err)
		assert.Equal(t, m.Height(0), uint8(17))
		assert.Equal(t, m.Blocker(0), true, "0x1201 carries the 0x200 bit")
		assert.Equal(t, m.Height(1), uint8(80))
		assert.Equal(t, m.Blocker(1), false, "0x400 alone is not the kept bit")
		assert.Equal(t, m.Height(height.Index(3, 2)), uint8(40))
		assert.Equal(t, m.Blocker(height.Index(3, 2)), true, "0x208 keeps only 0x200")
		assert.Equal(t, m.Height(height.Cells-1), uint8(255))
		assert.Equal(t, m.Blocker(height.Cells-1), true)
		// A cell nobody wrote: no height, no bit.
		assert.Equal(t, m.Height(2), uint8(0))
		assert.Equal(t, m.Blocker(2), false)
		// Out of range reads answer, never panic. The upper bound is the
		// one that bites: the engine's own index guard is `index <=
		// 2099200`, one PAST the last cell (re_docs/cell-grid.md), so a
		// caller that reproduces the comparison hands exactly Cells to
		// these and the bitset would index one word past its slice.
		assert.Equal(t, m.Height(height.Cells), uint8(0))
		assert.Equal(t, m.Height(-1), uint8(0))
		assert.Equal(t, m.Blocker(height.Cells), false)
		assert.Equal(t, m.Blocker(-1), false)
	})

	t.Run("short file", func(t *testing.T) {
		_, err := height.Decode(bytes.NewReader(make([]byte, height.FileBytes-1)))
		assert.ErrorContains(t, err, "read grid")
	})

	t.Run("trailing bytes", func(t *testing.T) {
		_, err := height.Decode(bytes.NewReader(make([]byte, height.FileBytes+1)))
		assert.ErrorContains(t, err, "trailing bytes")
	})
}
