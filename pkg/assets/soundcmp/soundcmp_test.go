// SPDX-License-Identifier: GPL-3.0-only

package soundcmp

import (
	"bytes"
	"encoding/binary"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// TestDecodeNameMatchesDoc verifies the rolling-XOR decode reproduces
// the worked example in re_docs/formats/cmp.md.
func TestDecodeNameMatchesDoc(t *testing.T) {
	enc := []byte{
		0x84, 0xde, 0xdc, 0xaf, 0x93, 0xd3, 0xff, 0xbb,
		0x99, 0x97, 0xd4, 0x84, 0xa6, 0xf5, 0x8e, 0x92,
		0xa6, 0xa3, 0xa8, 0x8f, 0xfd, 0xde, 0xcb, 0xa5,
		0x83, 0x91, 0xb2, 0xde, 0x96, 0xba, 0xa1,
	}
	assert.Check(t, cmp.Equal(decodeName(enc), "wav\\Ambience\\BehindDaBridge.ogg"))
}

// TestOpenAndRead builds a minimal Family-A archive with two entries
// and verifies index parsing + payload retrieval.
func TestOpenAndRead(t *testing.T) {
	p1 := []byte("hello")
	p2 := []byte("world!")

	names := []string{"foo", "BAR.dat"}
	indexBytes := func(name string, off, size int) []byte {
		nb := encodeName([]byte(name))
		var rec bytes.Buffer
		binary.Write(&rec, binary.LittleEndian, uint32(len(nb)))
		rec.Write(nb)
		rec.WriteByte(0)
		binary.Write(&rec, binary.LittleEndian, uint32(off))
		binary.Write(&rec, binary.LittleEndian, uint32(size))
		binary.Write(&rec, binary.LittleEndian, uint32(0))
		return rec.Bytes()
	}

	// First pass to size the index, then write with real offsets.
	rec0 := indexBytes(names[0], 0, len(p1))
	rec1 := indexBytes(names[1], 0, len(p2))
	indexLen := 4 + len(rec0) + len(rec1)
	off0 := indexLen
	off1 := off0 + len(p1)

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(2))
	buf.Write(indexBytes(names[0], off0, len(p1)))
	buf.Write(indexBytes(names[1], off1, len(p2)))
	buf.Write(p1)
	buf.Write(p2)

	a, err := Open(bytes.NewReader(buf.Bytes()))
	assert.NilError(t, err)
	assert.Assert(t, cmp.Len(a.Entries, 2))
	assert.Check(t, cmp.Equal(a.Entries[0].Name, "foo"))
	assert.Check(t, cmp.Equal(a.Entries[1].Name, "BAR.dat"))

	got, err := a.Read("foo")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(got), "hello"))

	got, err = a.Read("bar.DAT") // case-insensitive
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(got), "world!"))
}

func encodeName(plain []byte) []byte {
	out := make([]byte, len(plain))
	for i, b := range plain {
		out[i] = ^(b ^ key[i%32])
	}
	return out
}
