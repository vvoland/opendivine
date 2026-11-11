// SPDX-License-Identifier: GPL-3.0-only

// Package region reads Divine Divinity global/region.000 through region.007:
// named polygons grouped by subsystem, with a world ID on each record.
// See re_docs/formats/region.md for the layout reported by Raan.
package region

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const magicStr = "Divinity RegionsV1.0\x00"

// SubTag identifies the subsystem, not the file number or world ID.
type SubTag string

const (
	SubTagStory    SubTag = "StoryV1.0"
	SubTagMusic    SubTag = "MusicV1.0"
	SubTagMonster  SubTag = "MonsterV1.0"
	SubTagTreasure SubTag = "TreasureV1.0"
	SubTagReverb   SubTag = "ReverbV1.0"
	SubTagDungeon  SubTag = "DungeonV1.0"
	SubTagTraps    SubTag = "TrapsV1.0"
	SubTagShroud   SubTag = "ShroudV1.0"
)

// File holds one subsystem's named regions across worlds.
type File struct {
	Tag     SubTag
	Records []Record
}

// Record is a named polygon in one world.
// The meanings of Var0, Var1, Var2 and Flag remain unknown.
type Record struct {
	Var0, Var1, Var2 uint32
	WorldID          uint32
	Name             string
	Flag             uint8
	Vertices         []Vertex
}

// Vertex occupies 24 bytes on disk.
// X and Y are absolute global-map pixel coordinates, not cell coordinates.
// Opaque preserves the trailing engine data whose meaning remains unknown.
type Vertex struct {
	X, Y   float64
	Opaque [8]byte
}

// Decode reads an entire region file and rejects trailing bytes.
// Counts and string lengths are checked against the remaining data before
// allocating their decoded storage.
func Decode(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("region: read file: %w", err)
	}

	if !bytes.HasPrefix(data, []byte(magicStr)) {
		return nil, errors.New("region: bad magic")
	}

	buf := bytes.NewReader(data[len(magicStr):])

	tag, err := readNulString(buf)
	if err != nil {
		return nil, fmt.Errorf("region: sub-tag: %w", err)
	}

	switch SubTag(tag) {
	case SubTagStory, SubTagMusic, SubTagMonster, SubTagTreasure, SubTagReverb,
		SubTagDungeon, SubTagTraps, SubTagShroud:
	default:
		return nil, fmt.Errorf("region: unknown sub-tag %q", tag)
	}

	var count uint32
	if err := binary.Read(buf, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("region: count: %w", err)
	}
	// Even an unnamed polygon with no vertices needs 25 bytes.
	if uint64(count) > uint64(buf.Len()/25) {
		return nil, fmt.Errorf("region: count %d exceeds remaining data", count)
	}

	out := &File{Tag: SubTag(tag), Records: make([]Record, int(count))}
	for i := range out.Records {
		rec := &out.Records[i]

		var fields [4]uint32
		if err := binary.Read(buf, binary.LittleEndian, &fields); err != nil {
			return nil, fmt.Errorf("region: record %d fields: %w", i, err)
		}

		rec.Var0, rec.Var1, rec.Var2, rec.WorldID = fields[0], fields[1], fields[2], fields[3]

		rec.Name, err = readNulString(buf)
		if err != nil {
			return nil, fmt.Errorf("region: record %d name: %w", i, err)
		}

		rec.Flag, err = buf.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("region: record %d flag: %w", i, err)
		}

		var vertices uint32
		if err := binary.Read(buf, binary.LittleEndian, &vertices); err != nil {
			return nil, fmt.Errorf("region: record %d vertex count: %w", i, err)
		}

		if uint64(vertices) > uint64(buf.Len()/24) {
			return nil, fmt.Errorf("region: record %d vertex count %d exceeds remaining data", i, vertices)
		}

		rec.Vertices = make([]Vertex, int(vertices))
		if err := binary.Read(buf, binary.LittleEndian, rec.Vertices); err != nil {
			return nil, fmt.Errorf("region: record %d vertices: %w", i, err)
		}

		for j, v := range rec.Vertices {
			if math.IsNaN(v.X) || math.IsNaN(v.Y) || math.IsInf(v.X, 0) || math.IsInf(v.Y, 0) {
				return nil, fmt.Errorf("region: record %d vertex %d has non-finite coordinates", i, j)
			}
		}
	}

	if buf.Len() != 0 {
		return nil, fmt.Errorf("region: %d trailing bytes", buf.Len())
	}

	return out, nil
}

func readNulString(r *bytes.Reader) (string, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return "", err
	}

	if uint64(n) > uint64(r.Len()) {
		return "", fmt.Errorf("string length %d exceeds remaining data", n)
	}

	if n == 0 {
		return "", nil
	}

	buf := make([]byte, int(n))
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}

	if buf[len(buf)-1] != 0 {
		return "", errors.New("missing NUL terminator")
	}

	return string(buf[:len(buf)-1]), nil
}

// Contains reports whether an absolute pixel position is inside the polygon.
// It uses a nonzero winding rule, independent of vertex order, and includes
// points exactly on an edge or vertex.
// Boundary inclusion is a port convention, not verified engine behavior.
// The caller must select the matching WorldID before testing containment.
func (r *Record) Contains(x, y float64) bool {
	if len(r.Vertices) < 3 || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return false
	}

	winding := 0

	a := r.Vertices[len(r.Vertices)-1]
	for _, b := range r.Vertices {
		cross := (b.X-a.X)*(y-a.Y) - (x-a.X)*(b.Y-a.Y)
		if cross == 0 && x >= math.Min(a.X, b.X) && x <= math.Max(a.X, b.X) &&
			y >= math.Min(a.Y, b.Y) && y <= math.Max(a.Y, b.Y) {
			return true
		}

		if a.Y <= y && b.Y > y && cross > 0 {
			winding++
		} else if a.Y > y && b.Y <= y && cross < 0 {
			winding--
		}

		a = b
	}

	return winding != 0
}
