# `world.x<n>` — paged world cell-grid

The world map is split into **5 partitions** (`world.x0`..`world.x4`)
and within each partition into a **32 × 32 grid of fixed-size sectors**.
A sector covers `96 × 96` map cells; each cell is one byte (tile id).
Sectors may carry trailing per-sector metadata, so chunk size on disk
is variable.

All integers little-endian.

## Outer layout

```text
struct WorldFile {
    u32 chunk_offsets[1024];   // 32×32 sector grid; offsets are absolute byte offsets in this file
    u8  data[];                // sector chunks concatenated, addressed by chunk_offsets[i]
};
```

`chunk_offsets[0]` always equals `0x1000` (= `4 * 1024`, i.e. the size
of the offset table itself). Sector `i`'s payload occupies the file
range `[chunk_offsets[i], next)` where `next` is the smallest offset
greater than `chunk_offsets[i]` (typically `chunk_offsets[i+1]`, but
sectors are not strictly required to appear in order, and the final
sector ends at EOF).

## Sector chunk

Each sector covers **512 logical cells** (not 96×96 — that geometric
guess was wrong). The sector layout is a u16 pointer table followed
by per-cell records of variable length:

```text
struct Sector {
    u16   cell_offsets[512];   // 1024 bytes; each u16 is the byte offset (relative to the
                               //   start of the records area, i.e. just after this table)
                               //   of that cell's record.
    Record records[];          // variable count and total size
};

struct Record {
    i16   floor_tile_id;       // index into CPacked imagelist 2 (3363 64×64 floor tiles).
                               //   Most-common values across world.x0:
                               //   274 (134k cells, "void"), 0 (63k), 17 (grass), 100 (cobblestone).
                               //   88% of cells carry a non-zero floor id.
    i16   overlay_tile_id;     // secondary tile id; -1 means "no overlay"
                               //   (519k of 524k cells observed in world.x0).
    i16   _pad_off4;           // always 0 in shipped data
    u8    object_count;        // 0 = no per-cell objects (16-byte header still present)
    u8    flag_off7;           // bit 5 (0x20) = "skip this object" in the upgrade path
    i16   _pad_off8;           // always 0
    i16   header_h5;           // small enum, 15 distinct values; pairs with h6
    i16   header_h6;           // mirrors h5's distribution (likely a precomputed
                               //   adjacency / neighbour-mask used by the engine renderer)
    i16   _pad_off14;          // always 0
    Object objects[object_count];
};

struct Object {                // 8 bytes per object instance
    u32   xy_kind;             // bits  0..5 : sub_x  (0..63, added to cell_x)
                               // bits  6..11: sub_y  (0..63, added to cell_y)
                               // bits 12..15: per-object flags index — passed through
                               //              FUN_00581fa0 to derive runtime flags
                               // bits 16..31: unused / reserved
    u32   ord_kind;            // bits  0..9 : `param_4` arg to placer (probably stack height)
                               // bits 10..23: object catalogue id (param_5 to FUN_00572100)
};
```

The 9216-byte minimum is `1024 + 512 * 16` — 1024-byte pointer table
plus 512 empty records (16-byte header, zero objects). Sectors with
trailers carry one or more 8-byte object records appended to the
matching cells.

This account matches the parser at `div.exe:0x0059ce90`: the outer
loop walks the 1024 chunk-offsets table (`world.x*`), reads each
sector into a buffer, then iterates 512 cells per sector consuming
2 bytes of pointer-table per step and dispatching object placements
via `FUN_00572100`.

### Floor / overlay tile rendering

Each non-zero `floor_tile_id` indexes directly into **CPacked
imagelist 2** (`static\imagelists\CPackedb.2c` + `CPackedi.2c`,
3363 entries — all 64×64 RGB565 raw tiles, `flags=0`). Tile id
`0` and `274` are sentinels for "no floor" / "void" cells.

`overlay_tile_id`, when not `-1`, is a second tile drawn on top of
the floor (decals, road segments, stains). In shipped `world.x0`
about 4.8k of the 524k cells carry an overlay (most use values in
the 1978..1996 range — a small set of overlay-tile ids).

Together with the per-cell objects, these three layers
(floor → overlay → objects sorted by Layer/Y) give a complete
native-resolution render. There is **no** dependency on the
unshipped `dat\tiles.dat`; that file feeds a different code path
(the tile *manager* used by the editor / debug builds).

## Hex dump — `main/startup/world.x0` first offsets

```text
00000000  00 10 00 00                                      chunk_offsets[0]    = 0x00001000
00000004  00 34 00 00                                      chunk_offsets[1]    = 0x00003400
00000008  00 58 00 00                                      chunk_offsets[2]    = 0x00005800
0000000c  00 7c 00 00                                      chunk_offsets[3]    = 0x00007c00
00000010  00 a0 00 00                                      chunk_offsets[4]    = 0x0000a000
00000014  00 c4 00 00                                      chunk_offsets[5]    = 0x0000c400
00000018  08 e8 00 00                                      chunk_offsets[6]    = 0x0000e808  (← +0x2408 = 9224, irregular)
0000001c  20 0f 01 00                                      chunk_offsets[7]    = 0x00010f20
…
00001000  …                                                sector[0].cells[0..]
```

Sector 0..5 are the minimum 9216 bytes (offsets step by exactly
`0x2400`); sector 6 has an 8-byte trailer (step `0x2408`); etc.

## Loader citations

```text
div.exe:0x005a0300   FUN_005a0300   ctor: open file, read 0x400 u32 offset table into +0x7, append EOF as offsets[0x400]
div.exe:0x0059ce90   FUN_0059ce90   per-sector parser: walks 1024 sectors × 512 cells; pulls (sub_x, sub_y, obj_id, flags) from each record's object array and dispatches via FUN_00572100
div.exe:0x00572100   FUN_00572100   per-object placement: looks up sprite catalogue entry, applies adjustments, calls draw helpers
div.exe:0x0056d260   FUN_0056d260   per-cell height/flags stream loader (DAT_00750d64 path; usually `<save>\height.x<n>`)
```

Source path leak: `.\WORLD\World.cpp:0xce`.

Object layout used by the in-memory class (selected fields):

| Offset | Type | Meaning |
|---|---|---|
| `+0x04` | `char*` | strdup'd path |
| `+0x0c` | `FILE*` | file handle |
| `+0x1c` | `u32*` | offset table buffer (0x1004 bytes: 1024 offsets + appended file size) |
| `+0x20` | `u32` | sector grid stride along X (set to 128 by ctor) |
| `+0x24` | `u32` | sector grid stride along Y (= `param_4 * 2 + 0x40`; usually 128) |
| `+0x34` | `u32` | sector pixel size hint (16) |

## Companion partitions

Each `world.x<n>` is paired with `objects.x<n>`, `extfree.x<n>`,
`shroud.x<n>`, and a `mapv.<n>` version stamp. Their formats are not
yet reversed in detail.
