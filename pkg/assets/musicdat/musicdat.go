// SPDX-License-Identifier: GPL-3.0-only

// Package musicdat parses sound\music.dat. Format documented in
// re_docs/formats/sound.md.
//
// Currently exposes sections 1 (orchestrated tracks) and 2 (looped
// ambients); section 3 (per-region bindings) is not yet parsed.
package musicdat

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Track is one music or ambient entry.
type Track struct {
	Label    string
	Filename string
}

// File is a parsed sound\music.dat.
type File struct {
	Tracks   []Track // section 1
	Ambients []Track // section 2
}

// Decode parses music.dat
func Decode(r io.Reader) (*File, error) {
	out := File{}

	tracks, err := readSection(r, true)
	if err != nil {
		return nil, fmt.Errorf("music.dat section 1 (tracks): %w", err)
	}
	out.Tracks = tracks

	ambients, err := readSection(r, false)
	if err != nil {
		return nil, fmt.Errorf("music.dat section 2 (ambients): %w", err)
	}
	out.Ambients = ambients

	return &out, nil
}

// readSection parses one count-prefixed Track array. Section 1
// records carry a trailing u32 after the filename; section 2 does not.
func readSection(r io.Reader, withTrail bool) ([]Track, error) {
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, err
	}
	out := make([]Track, count)
	for i := range out {
		label, err := readLenStr(r)
		if err != nil {
			return nil, fmt.Errorf("entry %d label: %w", i, err)
		}
		fname, err := readLenStr(r)
		if err != nil {
			return nil, fmt.Errorf("entry %d filename: %w", i, err)
		}
		out[i] = Track{Label: label, Filename: fname}
		if withTrail {
			var trail uint32
			if err := binary.Read(r, binary.LittleEndian, &trail); err != nil {
				return nil, fmt.Errorf("entry %d trail: %w", i, err)
			}
		}
	}
	return out, nil
}

func readLenStr(r io.Reader) (string, error) {
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
	return string(buf), nil
}
