// SPDX-License-Identifier: GPL-3.0-only

// Package location reads Divine Divinity location.000/.001/.002 files:
// named coordinate tables tagged StoryV1.0, TrapsV1.0, or GenerationV1.0.
package location

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// magicStr is the fixed file header (22 chars + NUL = 23 bytes) at the
// start of every location.* file.
const magicStr = "Divinity LocationsV1.0\x00"

// Magic returns a fresh copy of the file header bytes.
func Magic() []byte { return []byte(magicStr) }

// SubTag identifies the variant of the file.
type SubTag string

const (
	SubTagStory      SubTag = "StoryV1.0"
	SubTagTraps      SubTag = "TrapsV1.0"
	SubTagGeneration SubTag = "GenerationV1.0"
)

// Record is one row of the table.
type Record struct {
	V0, V1, V3 uint32
	Name       string
}

// File is the parsed contents of a location.* file.
type File struct {
	Tag     SubTag
	Records []Record
}

// Errors reported by this package.
var (
	ErrBadMagic   = errors.New("location: bad magic")
	ErrBadSubTag  = errors.New("location: unknown sub-tag")
	ErrTruncated  = errors.New("location: truncated")
	ErrBadNameLen = errors.New("location: bad name length")
)

// Decode parses a location.* file from r.
func Decode(r io.Reader) (*File, error) {
	br := bufReader(r)
	magic := make([]byte, len(magicStr))
	if _, err := io.ReadFull(br, magic); err != nil {
		return nil, fmt.Errorf("location: read magic: %w", err)
	}
	if !bytes.Equal(magic, []byte(magicStr)) {
		return nil, fmt.Errorf("%w: have %q want %q", ErrBadMagic, magic, magicStr)
	}
	tag, err := readNulString(br)
	if err != nil {
		return nil, fmt.Errorf("location: sub-tag: %w", err)
	}
	switch SubTag(tag) {
	case SubTagStory, SubTagTraps, SubTagGeneration:
		// ok
	default:
		return nil, fmt.Errorf("%w: %q", ErrBadSubTag, tag)
	}

	var count uint32
	if err := binary.Read(br, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("location: count: %w", err)
	}
	records := make([]Record, count)
	for i := range records {
		var fields [3]uint32
		if err := binary.Read(br, binary.LittleEndian, &fields); err != nil {
			return nil, fmt.Errorf("location: record %d fields: %w", i, err)
		}
		records[i].V0 = fields[0]
		records[i].V1 = fields[1]
		records[i].V3 = fields[2]
		name, err := readNulString(br)
		if err != nil {
			return nil, fmt.Errorf("location: record %d name: %w", i, err)
		}
		records[i].Name = name
	}
	return &File{Tag: SubTag(tag), Records: records}, nil
}

// Encode writes f to w using the location.* layout.
func Encode(w io.Writer, f *File) error {
	switch f.Tag {
	case SubTagStory, SubTagTraps, SubTagGeneration:
	default:
		return fmt.Errorf("%w: %q", ErrBadSubTag, f.Tag)
	}
	if _, err := w.Write([]byte(magicStr)); err != nil {
		return err
	}
	if err := writeNulString(w, string(f.Tag)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(f.Records))); err != nil {
		return err
	}
	for i, rec := range f.Records {
		if err := binary.Write(w, binary.LittleEndian, [3]uint32{rec.V0, rec.V1, rec.V3}); err != nil {
			return fmt.Errorf("location: record %d fields: %w", i, err)
		}
		if err := writeNulString(w, rec.Name); err != nil {
			return fmt.Errorf("location: record %d name: %w", i, err)
		}
	}
	return nil
}

// readNulString reads a u32 length (counting the trailing NUL) followed by
// that many bytes; returns the string with the NUL stripped. Length 0
// is permitted and returns "".
func readNulString(r io.Reader) (string, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	if buf[n-1] != 0 {
		return "", fmt.Errorf("%w: missing NUL terminator", ErrBadNameLen)
	}
	return string(buf[:n-1]), nil
}

// writeNulString emits the same encoding readNulString consumes.
func writeNulString(w io.Writer, s string) error {
	if s == "" {
		return binary.Write(w, binary.LittleEndian, uint32(0))
	}
	n := uint32(len(s) + 1)
	if err := binary.Write(w, binary.LittleEndian, n); err != nil {
		return err
	}
	if _, err := w.Write([]byte(s)); err != nil {
		return err
	}
	_, err := w.Write([]byte{0})
	return err
}

// bufReader wraps r in a buffered reader unless it already supports byte
// reads efficiently. binary.Read on an unbuffered file does many tiny
// syscalls; this avoids that.
func bufReader(r io.Reader) io.Reader {
	type byteReader interface {
		io.Reader
		ReadByte() (byte, error)
	}
	if _, ok := r.(byteReader); ok {
		return r
	}
	return &readerWrap{r: r}
}

type readerWrap struct {
	r   io.Reader
	buf [1]byte
}

func (b *readerWrap) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *readerWrap) ReadByte() (byte, error) {
	_, err := io.ReadFull(b.r, b.buf[:])
	return b.buf[0], err
}
