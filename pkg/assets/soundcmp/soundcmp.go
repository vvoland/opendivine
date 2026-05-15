// SPDX-License-Identifier: GPL-3.0-only

// Package soundcmp reads Family-A `.cmp` archives: name-indexed
// blob containers used by dat\sound.cmp, dat\flat.cmp, dat\global.cmp,
// localizations\<lang>\text.cmp, dat\mono.cmp, and
// localizations\<lang>\mono.cmp.
//
// Format and obfuscation scheme are documented in
// re_docs/formats/cmp.md
package soundcmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// key is the 32-byte rolling XOR used to obfuscate entry names; see
// re_docs/formats/cmp.md §"Name obfuscation".
var key = [32]byte{
	0x0c, 0x40, 0x55, 0x0c, 0x2d, 0x41, 0x62, 0x2d,
	0x03, 0x06, 0x48, 0x1e, 0x05, 0x48, 0x14, 0x05,
	0x30, 0x32, 0x33, 0x34, 0x63, 0x63, 0x46, 0x33,
	0x18, 0x09, 0x28, 0x0f, 0x06, 0x22, 0x39, 0x17,
}

// Entry is one index record. Offset is absolute (from file start).
type Entry struct {
	Name   string
	Offset int64
	Size   int64
	Flags  uint32
}

// Archive is a parsed Family-A `.cmp` index. Use Open or Read to fetch
// a payload by name.
type Archive struct {
	r       io.ReaderAt
	Entries []Entry
	byName  map[string]int
}

// Open reads the index from r.
// The index sits at the start of the file and references payloads later
// in the same file via absolute offsets.
func Open(r io.ReaderAt) (*Archive, error) {
	hdr := make([]byte, 4)
	if _, err := r.ReadAt(hdr, 0); err != nil {
		return nil, fmt.Errorf("read count: %w", err)
	}
	count := binary.LittleEndian.Uint32(hdr)
	a := &Archive{r: r, Entries: make([]Entry, count), byName: make(map[string]int, count)}

	pos := int64(4)
	tmp := make([]byte, 256)
	for i := range count {
		// name_len
		if _, err := r.ReadAt(hdr, pos); err != nil {
			return nil, fmt.Errorf("entry %d name_len: %w", i, err)
		}
		nl := int64(binary.LittleEndian.Uint32(hdr))
		pos += 4
		if nl > int64(cap(tmp)) {
			tmp = make([]byte, nl)
		}
		nb := tmp[:nl]
		if _, err := r.ReadAt(nb, pos); err != nil {
			return nil, fmt.Errorf("entry %d name: %w", i, err)
		}
		pos += nl
		// trailing NUL is present iff name_len > 0
		if nl > 0 {
			pos++
		}
		// offset, size, flags
		trailer := make([]byte, 12)
		if _, err := r.ReadAt(trailer, pos); err != nil {
			return nil, fmt.Errorf("entry %d trailer: %w", i, err)
		}
		pos += 12

		name := decodeName(nb)
		a.Entries[i] = Entry{
			Name:   name,
			Offset: int64(binary.LittleEndian.Uint32(trailer[0:4])),
			Size:   int64(binary.LittleEndian.Uint32(trailer[4:8])),
			Flags:  binary.LittleEndian.Uint32(trailer[8:12]),
		}
		a.byName[normalize(name)] = int(i)
	}
	return a, nil
}

// Read returns the raw payload bytes for the named entry.
// Names are matched case-insensitively after backslash/forward-slash
// normalisation, so callers may use either separator.
func (a *Archive) Read(name string) ([]byte, error) {
	idx, ok := a.byName[normalize(name)]
	if !ok {
		return nil, fmt.Errorf("soundcmp: entry %q not found", name)
	}
	e := a.Entries[idx]
	buf := make([]byte, e.Size)
	if _, err := a.r.ReadAt(buf, e.Offset); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("soundcmp: read %q: %w", name, err)
	}
	return buf, nil
}

func decodeName(enc []byte) string {
	out := make([]byte, len(enc))
	for i, b := range enc {
		out[i] = (^b) ^ key[i%32]
	}
	return string(out)
}

// normalize uppercases and converts forward slashes to backslashes so
// "wav/music/1.ogg" and "WAV\\Music\\1.ogg" hash to the same slot.
func normalize(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "/", "\\"))
}
