// SPDX-License-Identifier: GPL-3.0-only

// Package world reads Divine Divinity's world.x<n> partitions:
// the per-region object-instance grid that also carries each cell's
// floor-tile id and overlay-tile id in its 16-byte cell header.
package world

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Constants matching the engine's world layout.
const (
	SectorCount       = 1024
	CellsPerSector    = 512
	CellHeaderBytes   = 16
	CellPointerBytes  = 2
	PointerTableBytes = CellsPerSector * CellPointerBytes // 1024
	ObjectBytes       = 8
	CellPxStride      = 0x40 // 64 — engine's per-cell coord step inside a sector
)

// Cell is the decoded per-cell record.
type Cell struct {
	// Header (8 shorts at the start of the record):
	FloorTileID  int16 // short[0] — index into CPacked imagelist 2 (3363 64×64 floor tiles)
	OverlayTile  int16 // short[1] — secondary tile id; -1 means "no overlay"
	HeaderField2 int16 // short[2] — always 0 in observed worlds
	ObjectCount  uint8 // byte 6
	HeaderByte7  uint8 // byte 7
	HeaderField4 int16 // short[4] — always 0
	HeaderField5 int16 // short[5] — small enum, 15 values; pairs with field6
	HeaderField6 int16 // short[6] — same distribution as field5
	HeaderField7 int16 // short[7] — always 0

	// Per-cell objects, decoded from the 8-byte (xy_kind, ord_kind) pair.
	Objects []Object
}

// Object is one placed entity at a cell.
type Object struct {
	SubX, SubY  uint8  // sub-cell offsets, 0..63
	FlagIndex   uint8  // bits 12..15 of xy_kind — passed through FUN_00581fa0 to derive runtime flags
	CatalogueID uint16 // 14-bit obj_id into objects.000 / CPacked imagelist 0
	Layer       uint16 // 10-bit ord_kind low bits — painter's-algorithm key
}

// Walk decodes every populated cell in a world.x<n> file. cb receives
// the cell coordinates (cellX/cellY in the engine's native units;
// each cell is 64 world coord units wide) plus the parsed Cell.
//
// cellX / cellY are the engine's native coords matching the per-object
// formula (xy_kind & 0x3f) + cellIdx * 0x40 / ((xy_kind >> 6) & 0x3f) +
// sectorIdx * 0x40, so multiplying by CellPxStride / 64 = 1 keeps them
// directly comparable with object positions.
func Walk(world []byte, cb func(cellX, cellY int, c Cell)) error {
	if len(world) < 4*SectorCount {
		return fmt.Errorf("world: file too short for offset table (%d bytes)", len(world))
	}
	chunk := make([]uint32, SectorCount)
	for i := range chunk {
		chunk[i] = binary.LittleEndian.Uint32(world[4*i:])
	}
	for s := range SectorCount {
		start := chunk[s]
		var end uint32
		if s+1 < SectorCount {
			end = chunk[s+1]
		} else {
			end = uint32(len(world))
		}
		if int(end) > len(world) || start >= end {
			continue
		}
		sec := world[start:end]
		if len(sec) <= PointerTableBytes {
			continue
		}
		yBase := s * CellPxStride
		for cellIdx := range CellsPerSector {
			recOff := int(binary.LittleEndian.Uint16(sec[cellIdx*CellPointerBytes:]))
			rec := PointerTableBytes + recOff
			if rec+CellHeaderBytes > len(sec) {
				continue
			}
			c := Cell{
				FloorTileID:  int16(binary.LittleEndian.Uint16(sec[rec : rec+2])),
				OverlayTile:  int16(binary.LittleEndian.Uint16(sec[rec+2 : rec+4])),
				HeaderField2: int16(binary.LittleEndian.Uint16(sec[rec+4 : rec+6])),
				ObjectCount:  sec[rec+6],
				HeaderByte7:  sec[rec+7],
				HeaderField4: int16(binary.LittleEndian.Uint16(sec[rec+8 : rec+10])),
				HeaderField5: int16(binary.LittleEndian.Uint16(sec[rec+10 : rec+12])),
				HeaderField6: int16(binary.LittleEndian.Uint16(sec[rec+12 : rec+14])),
				HeaderField7: int16(binary.LittleEndian.Uint16(sec[rec+14 : rec+16])),
			}
			if int(c.ObjectCount) > 0 {
				objBase := rec + CellHeaderBytes
				if objBase+int(c.ObjectCount)*ObjectBytes > len(sec) {
					return fmt.Errorf("world: sector %d cell %d truncated (count=%d)", s, cellIdx, c.ObjectCount)
				}
				c.Objects = make([]Object, c.ObjectCount)
				for j := range c.ObjectCount {
					o := objBase + int(j)*ObjectBytes
					xy := binary.LittleEndian.Uint32(sec[o : o+4])
					ord := binary.LittleEndian.Uint32(sec[o+4 : o+8])
					c.Objects[j] = Object{
						SubX:        uint8(xy & 0x3f),
						SubY:        uint8((xy >> 6) & 0x3f),
						FlagIndex:   uint8((xy >> 12) & 0xf),
						CatalogueID: uint16((ord >> 10) & 0x3fff),
						Layer:       uint16(ord & 0x3ff),
					}
				}
			}
			cellX := cellIdx*CellPxStride + 0 // sub-x is added per object
			cellY := yBase
			cb(cellX, cellY, c)
		}
	}
	return nil
}

// ReadAll loads a world partition entirely into memory and returns
// every populated cell as a flat slice. Use Walk for streaming.
type Populated struct {
	X, Y int // sector-anchored cell coords (object positions add SubX/SubY)
	Cell Cell
}

func ReadAll(r io.Reader) ([]Populated, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var out []Populated
	err = Walk(buf, func(x, y int, c Cell) {
		if c.FloorTileID == 0 && c.OverlayTile == -1 && c.ObjectCount == 0 {
			return // empty cell
		}
		out = append(out, Populated{X: x, Y: y, Cell: c})
	})
	return out, err
}
