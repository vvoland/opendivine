// SPDX-License-Identifier: GPL-3.0-only

// Package musicdat parses sound/music.dat: tracks, ambients and zone bindings.
// The five-list binding layout is documented in re_docs/formats/sound.md.
package musicdat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Track is one music or ambient entry.
type Track struct {
	Label    string
	Filename string
}

// Assignment binds a track label to its playback volume, not a pick weight.
type Assignment struct {
	Label  string
	Volume float32
}

// Zone binds a MusicV1.0 region name to five parallel track lists.
// Lists 0, 1 and 2 hold primary, reserved and override music respectively.
// Lists 3 and 4 hold day and night ambients.
// Flag preserves an unknown field, observed as 0 or 1 in shipped data.
type Zone struct {
	Label string
	Lists [5][]Assignment
	Flag  uint32
}

// File is a parsed sound/music.dat.
type File struct {
	Tracks   []Track // section 1
	Ambients []Track // section 2
	Zones    []Zone  // section 3
}

// Decode parses all three sections and rejects trailing data.
// Counts and string lengths are bounded by the remaining input before allocation.
func Decode(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("music.dat: read file: %w", err)
	}

	buf := bytes.NewReader(data)

	tracks, err := readSection(buf, true)
	if err != nil {
		return nil, fmt.Errorf("music.dat section 1 (tracks): %w", err)
	}

	ambients, err := readSection(buf, false)
	if err != nil {
		return nil, fmt.Errorf("music.dat section 2 (ambients): %w", err)
	}

	count, err := readCount(buf, 28) // label length, five list counts and flag
	if err != nil {
		return nil, fmt.Errorf("music.dat section 3 (zones): %w", err)
	}

	zones := make([]Zone, count)
	for i := range zones {
		zone := &zones[i]

		zone.Label, err = readLenStr(buf)
		if err != nil {
			return nil, fmt.Errorf("music.dat zone %d label: %w", i, err)
		}

		for j := range zone.Lists {
			count, err := readCount(buf, 8) // label length and volume
			if err != nil {
				return nil, fmt.Errorf("music.dat zone %d list %d: %w", i, j, err)
			}

			zone.Lists[j] = make([]Assignment, count)
			for k := range zone.Lists[j] {
				entry := &zone.Lists[j][k]

				entry.Label, err = readLenStr(buf)
				if err != nil {
					return nil, fmt.Errorf("music.dat zone %d list %d entry %d: %w", i, j, k, err)
				}

				if err := binary.Read(buf, binary.LittleEndian, &entry.Volume); err != nil {
					return nil, fmt.Errorf("music.dat zone %d list %d volume: %w", i, j, err)
				}

				if math.IsNaN(float64(entry.Volume)) || entry.Volume < 0 || entry.Volume > 1 {
					return nil, fmt.Errorf("music.dat zone %d list %d: invalid volume %v", i, j, entry.Volume)
				}
			}
		}

		if err := binary.Read(buf, binary.LittleEndian, &zone.Flag); err != nil {
			return nil, fmt.Errorf("music.dat zone %d flag: %w", i, err)
		}
	}

	if buf.Len() != 0 {
		return nil, fmt.Errorf("music.dat: %d trailing bytes", buf.Len())
	}

	return &File{Tracks: tracks, Ambients: ambients, Zones: zones}, nil
}

// Section 1 records carry a trailing u32; section 2 does not.
func readSection(r *bytes.Reader, withTrail bool) ([]Track, error) {
	minBytes := 8
	if withTrail {
		minBytes = 12
	}

	count, err := readCount(r, minBytes)
	if err != nil {
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

func readCount(r *bytes.Reader, minBytes int) (int, error) {
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return 0, err
	}

	if uint64(count) > uint64(r.Len()/minBytes) {
		return 0, fmt.Errorf("count %d exceeds remaining data", count)
	}

	return int(count), nil
}

func readLenStr(r *bytes.Reader) (string, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return "", err
	}

	if uint64(n) > uint64(r.Len()) {
		return "", fmt.Errorf("string length %d exceeds remaining data", n)
	}

	buf := make([]byte, int(n))
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}

	return string(buf), nil
}
