// SPDX-License-Identifier: GPL-3.0-only

package region

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"grono.dev/opendivine/internal/testutils"
)

// syntheticFile writes the reported field order explicitly rather than using
// a matching encoder that could hide the same layout mistake as the decoder.
func syntheticFile(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	for _, v := range []any{
		[]byte("Divinity RegionsV1.0\x00"),
		uint32(10), []byte("StoryV1.0\x00"), uint32(2),
		[4]uint32{0x12345678, 2, 3, 4},
		uint32(5), []byte("zone\x00"), uint8(7), uint32(3),
		float64(1024.25), float64(2048.5), [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		float64(1088.75), float64(2048.5), [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
		float64(1024.25), float64(2112.125), [8]byte{0xff},
		[4]uint32{9, 10, 11, 0}, uint32(0), uint8(0), uint32(0),
	} {
		assert.NilError(t, binary.Write(&buf, binary.LittleEndian, v))
	}

	return buf.Bytes()
}

// TestDecode pins the unaligned vertex count, float64 coordinates, 24-byte
// stride and immediate next-record framing, including an empty name/polygon.
func TestDecode(t *testing.T) {
	t.Parallel()

	out, err := Decode(bytes.NewReader(syntheticFile(t)))
	assert.NilError(t, err)
	assert.Assert(t, is.DeepEqual(out, &File{
		Tag: SubTagStory,
		Records: []Record{
			{
				Var0: 0x12345678, Var1: 2, Var2: 3, WorldID: 4, Name: "zone", Flag: 7,
				Vertices: []Vertex{
					{X: 1024.25, Y: 2048.5, Opaque: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}},
					{X: 1088.75, Y: 2048.5, Opaque: [8]byte{8, 7, 6, 5, 4, 3, 2, 1}},
					{X: 1024.25, Y: 2112.125, Opaque: [8]byte{0xff}},
				},
			},
			{Var0: 9, Var1: 10, Var2: 11, Vertices: []Vertex{}},
		},
	}))
}

// TestRejectMalformed guards against partial records, count-based allocations
// beyond the input, invalid strings and silently accepting a wrong stride.
func TestRejectMalformed(t *testing.T) {
	t.Parallel()

	data := syntheticFile(t)
	for n := range data {
		_, err := Decode(bytes.NewReader(data[:n]))
		assert.Assert(t, err != nil, "accepted truncation at byte %d", n)
	}

	for _, tc := range []struct {
		name   string
		offset int
		bytes  []byte
		want   string
	}{
		{"bad magic", 0, []byte{'X'}, "bad magic"},
		{"unknown subsystem", 25, []byte("WrongV1.0\x00"), "unknown sub-tag"},
		{"unterminated subsystem", 34, []byte{'X'}, "missing NUL"},
		{"oversized subsystem", 21, []byte{0xff, 0xff, 0xff, 0xff}, "string length"},
		{"oversized record count", 35, []byte{0xff, 0xff, 0xff, 0xff}, "count"},
		{"oversized name", 55, []byte{0xff, 0xff, 0xff, 0xff}, "name: string length"},
		{"unterminated name", 63, []byte{'X'}, "name: missing NUL"},
		{"oversized vertex count", 65, []byte{0xff, 0xff, 0xff, 0xff}, "vertex count"},
		{"non-finite coordinate", 69, binary.LittleEndian.AppendUint64(nil, math.Float64bits(math.Inf(1))), "non-finite"},
		{"trailing bytes", len(data), []byte{0}, "trailing bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bad := make([]byte, max(len(data), tc.offset+len(tc.bytes)))
			copy(bad, data)
			copy(bad[tc.offset:], tc.bytes)
			_, err := Decode(bytes.NewReader(bad))
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

// TestContains covers concavity and reversed winding, which a bounding-box or
// orientation-dependent test would get wrong, plus the chosen boundary rule.
func TestContains(t *testing.T) {
	t.Parallel()

	vertices := []Vertex{
		{X: 1000, Y: 2000}, {X: 1010, Y: 2000}, {X: 1010, Y: 2004},
		{X: 1004, Y: 2004}, {X: 1004, Y: 2010}, {X: 1000, Y: 2010},
	}

	for _, tc := range []struct {
		name string
		x, y float64
		want bool
	}{
		{"inside", 1002.5, 2008.5, true},
		{"outside", 999, 2005, false},
		{"concave cutout", 1008, 2008, false},
		{"edge", 1007, 2004, true},
		{"vertex", 1004, 2004, true},
		{"nan", math.NaN(), 2000, false},
		{"infinity", 1000, math.Inf(1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := Record{Vertices: slices.Clone(vertices)}
			assert.Equal(t, rec.Contains(tc.x, tc.y), tc.want)
			slices.Reverse(rec.Vertices)
			assert.Equal(t, rec.Contains(tc.x, tc.y), tc.want)
		})
	}

	for n := range 3 {
		rec := Record{Vertices: vertices[:n]}
		assert.Equal(t, rec.Contains(1000, 2000), false)
	}
}

// TestShippedFiles checks all eight subsystem tables, including exact stream
// consumption in Decode, instead of relying on heuristic name scanning.
func TestShippedFiles(t *testing.T) {
	t.Parallel()

	gamedata := testutils.TestGameData(t)
	for n, tc := range []struct {
		tag   SubTag
		count int
	}{
		{SubTagStory, 435}, {SubTagMusic, 155}, {SubTagMonster, 111},
		{SubTagTreasure, 1}, {SubTagReverb, 89}, {SubTagDungeon, 90},
		{SubTagTraps, 81}, {SubTagShroud, 90},
	} {
		t.Run(fmt.Sprintf("region.%03d", n), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(filepath.Join(gamedata, "global", fmt.Sprintf("region.%03d", n)))
			assert.NilError(t, err)
			out, err := Decode(bytes.NewReader(data))
			assert.NilError(t, err)
			assert.Equal(t, out.Tag, tc.tag)
			assert.Equal(t, len(out.Records), tc.count)
		})
	}
}
