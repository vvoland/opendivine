// SPDX-License-Identifier: GPL-3.0-only

package musicdat

import (
	"bytes"
	"encoding/binary"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// TestDecodeRoundTrip builds a synthetic music.dat (2 tracks, 1
// ambient) and verifies Decode reproduces the input.
func TestDecodeRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	// Section 1: 2 tracks
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(2)))
	writeStr(t, &buf, "1")
	writeStr(t, &buf, "1.ogg")
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(0)))
	writeStr(t, &buf, "17cAmMix2")
	writeStr(t, &buf, "17cAmMix2.ogg")
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(0)))
	// Section 2: 1 ambient
	assert.NilError(t, binary.Write(&buf, binary.LittleEndian, uint32(1)))
	writeStr(t, &buf, "BehindDaBridge")
	writeStr(t, &buf, "BehindDaBridge.ogg")

	got, err := Decode(&buf)
	assert.NilError(t, err)
	assert.Assert(t, cmp.Len(got.Tracks, 2))
	assert.Check(t, cmp.Equal(got.Tracks[0].Label, "1"))
	assert.Check(t, cmp.Equal(got.Tracks[0].Filename, "1.ogg"))
	assert.Check(t, cmp.Equal(got.Tracks[1].Label, "17cAmMix2"))
	assert.Check(t, cmp.Equal(got.Tracks[1].Filename, "17cAmMix2.ogg"))
	assert.Assert(t, cmp.Len(got.Ambients, 1))
	assert.Check(t, cmp.Equal(got.Ambients[0].Label, "BehindDaBridge"))
}

func writeStr(t *testing.T, buf *bytes.Buffer, s string) {
	t.Helper()
	assert.NilError(t, binary.Write(buf, binary.LittleEndian, uint32(len(s))))
	_, err := buf.WriteString(s)
	assert.NilError(t, err)
}
