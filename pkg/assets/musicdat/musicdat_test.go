// SPDX-License-Identifier: GPL-3.0-only

package musicdat

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"grono.dev/opendivine/internal/testutils"
)

func syntheticMusicDat(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(2)))
	writeStr(t, &buf, "1")
	writeStr(t, &buf, "1.ogg")
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(0)))
	writeStr(t, &buf, "17cAmMix2")
	writeStr(t, &buf, "17cAmMix2.ogg")
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(0)))
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(1)))
	writeStr(t, &buf, "BehindDaBridge")
	writeStr(t, &buf, "BehindDaBridge.ogg")

	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(2)))
	writeStr(t, &buf, "Aleroth")

	for _, list := range [5][]Assignment{
		{{Label: "1", Volume: 0.2}, {Label: "17cAmMix2", Volume: 0.8}},
		{{Label: "reserved", Volume: 0.3}},
		{{Label: "override", Volume: 0.4}},
		{{Label: "day", Volume: 0.5}},
		{{Label: "night", Volume: 0.6}},
	} {
		assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(len(list))))

		for _, entry := range list {
			writeStr(t, &buf, entry.Label)
			assert.NilError(t, binary.Write(&buf, binary.LittleEndian, entry.Volume))
		}
	}

	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(0)))
	writeStr(t, &buf, "Empty")
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, [6]uint32{0, 0, 0, 0, 0, 1}))

	return buf.Bytes()
}

// Keep catalogue decoding covered while pinning all five binding lists and
// the flag that separates consecutive zones.
func TestDecode(t *testing.T) {
	t.Parallel()

	got, err := Decode(bytes.NewReader(syntheticMusicDat(t)))
	assert.NilError(t, err)
	assert.Assert(t, is.DeepEqual(got.Tracks, []Track{{"1", "1.ogg"}, {"17cAmMix2", "17cAmMix2.ogg"}}))
	assert.Assert(t, is.DeepEqual(got.Ambients, []Track{{"BehindDaBridge", "BehindDaBridge.ogg"}}))
	assert.Equal(t, len(got.Zones), 2)
	assert.Equal(t, got.Zones[0].Label, "Aleroth")
	assert.Assert(t, is.DeepEqual(got.Zones[0].Lists, [5][]Assignment{
		{{Label: "1", Volume: 0.2}, {Label: "17cAmMix2", Volume: 0.8}},
		{{Label: "reserved", Volume: 0.3}},
		{{Label: "override", Volume: 0.4}},
		{{Label: "day", Volume: 0.5}},
		{{Label: "night", Volume: 0.6}},
	}))
	assert.Equal(t, got.Zones[0].Flag, uint32(0))
	assert.Equal(t, got.Zones[1].Label, "Empty")
	assert.Equal(t, got.Zones[1].Flag, uint32(1))

	for _, list := range got.Zones[1].Lists {
		assert.Equal(t, len(list), 0)
	}
}

// Reject partial bindings, trailing bytes and allocation-driving lengths or
// counts larger than the input, rather than silently stopping at section 2.
func TestRejectMalformed(t *testing.T) {
	t.Parallel()

	data := syntheticMusicDat(t)
	for n := range data {
		_, err := Decode(bytes.NewReader(data[:n]))
		assert.Assert(t, err != nil, "accepted truncation at byte %d", n)
	}

	_, err := Decode(bytes.NewReader(append(bytes.Clone(data), 0)))
	assert.ErrorContains(t, err, "trailing bytes")

	listOffset := bytes.Index(data, []byte("Aleroth")) + len("Aleroth")
	for _, tc := range []struct {
		name   string
		offset int
		value  uint32
		want   string
	}{
		{"track count", 0, math.MaxUint32, "count"},
		{"string length", 4, math.MaxUint32, "string length"},
		{"binding count", listOffset, math.MaxUint32, "list 0"},
		{"nan volume", listOffset + 9, math.Float32bits(float32(math.NaN())), "invalid volume"},
		{"excess volume", listOffset + 9, math.Float32bits(1.1), "invalid volume"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bad := bytes.Clone(data)
			binary.LittleEndian.PutUint32(bad[tc.offset:], tc.value)
			_, err := Decode(bytes.NewReader(bad))
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestShippedFile(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(testutils.TestGameData(t), "sound", "music.dat"))
	assert.NilError(t, err)
	got, err := Decode(bytes.NewReader(data))
	assert.NilError(t, err)
	assert.Equal(t, len(got.Tracks), 43)
	assert.Equal(t, len(got.Ambients), 22)
	assert.Equal(t, len(got.Zones), 153)
	assert.Equal(t, got.Zones[0].Label, "Aleroth")
	assert.Assert(t, is.DeepEqual(got.Zones[0].Lists[0], []Assignment{{Label: "4", Volume: 0.2}, {Label: "forest", Volume: 0.2}}))
}

func writeStr(t *testing.T, buf *bytes.Buffer, s string) {
	t.Helper()
	assert.NilError(t, binary.Write(buf, binary.LittleEndian, uint32(len(s))))
	_, err := buf.WriteString(s)
	assert.NilError(t, err)
}
