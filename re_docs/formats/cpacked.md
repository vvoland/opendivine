# `static\imagelists\CPacked*.<n>c` — compressed cell / sprite imagelist

The engine's main image bank: 13 paired blob (`CPackedb.<n>c`) + index
(`CPackedi.<n>c`) files holding all cell sprites used to render the
world (terrain tiles, props, walls, etc.) and other 2D imagery
(plates, icons). The non-`c` siblings (`Packedb.<n>` / `Packedi.<n>`)
are the uncompressed editor-build variants and are not shipped.

All integers little-endian.

## Top-level pipeline

```text
CPackedi.<n>c (index)  → 56-byte entry per cell, gives blob_offset
CPackedb.<n>c (blob)   → at blob_offset: u32 uncomp_size + LZO1X-1 stream
LZO1X-1 decompress     → produces the cell's "raw" payload (header + pixel data)
post-decode (engine)   → applies per-flag fix-ups (tint, transparency mask, …)
```

The decompressor is the same one used by automap tiles
(`div.exe:0x005a4ce0`), via `github.com/anchore/go-lzo`. Every shipped
entry observed decodes cleanly.

## Index — `CPackedi.<n>c`

```text
struct PackedIndex {
    Entry entries[N];              // file size = 56 * N (3604..7208 entries per file)
};

struct Entry {
    u32 blob_offset;               // absolute byte offset into the paired blob file
    u32 width;                     // cell width in pixels (e.g. 170)
    u32 height;                    // cell height in pixels (e.g. 127)
    u32 flags;                     // bit 0x01 = standard sprite path (most cells); bit 0x10 = special
    u32 anchor_x;                  // NOT the render anchor: a near-constant ~2px
    u32 anchor_y;                  // encoder inset (anchor_x is 1..26, almost always 2,
                                   // across all 7208 CPacked.0 entries; anchor_y is 0 for
                                   // ~75%).  The object blit (FUN_00556a80) reads the cell
                                   // payload's width/height (+0x8/+0xa) for clipping and
                                   // takes the destination top-left from its caller; it
                                   // never reads these index fields.  Objects render with
                                   // top-left at world (X, Y) (see render-trace.md), so
                                   // subtracting anchor_x/anchor_y just shifts every sprite
                                   // ~2px and is wrong.
    u32 width_minus_1;             // = width  - 1
    u32 height_minus_1;            // = height - 1
    u32 packed_dims;               // (sub_height << 16) | sub_width — interpretation TBD.
                                   // Roughly tracks (width/2, height/2..0.76*height) but is
                                   // NOT the engine's render anchor (verified empirically: applying
                                   // it as a hotspot mis-positions environment objects).
                                   // Likely a tight visible-content bbox or animation pivot.
    u32 reserved[5];               // zero in every shipped entry observed
};
```

The compressed-blob *size* of entry `i` is `entry[i+1].blob_offset -
entry[i].blob_offset`; the last entry runs to EOF of the blob file.

## Blob — `CPackedb.<n>c`

```text
At each `entry.blob_offset`:
    u32 uncompressed_size;         // size of the post-LZO payload
    u8  lzo1x_stream[blob_size - 4]; // LZO1X-1 compressed payload
```

Decompression is **stock LZO1X-1** — feed `lzo1x_stream` to any
LZO1X-1 decoder with output capacity `uncompressed_size`.

## Decompressed cell payload — sprite encoding

After LZO decompression a `flags & 0x01`-set cell yields a sparse
RGB565 sprite stored as a per-line span table. Pixels not covered by
any span are transparent.

```text
struct Cell {
    u32   total_size;         // = LZO uncompressed_size
    u32   pixel_data_offset;  // byte offset (from start of payload) where the packed RGB565 pixel runs begin
    u16   width;              // matches the index entry (e.g. 170)
    u16   height;             // matches the index entry; also serves as num_lines
    Line  lines[height];      // exactly `height` entries — one per scan line, top → bottom
    u8    pixel_data[];       // RGB565 pixel runs, referenced by Line.pixel_offset, span lengths
};

struct Line {
    // Empty line: 8 bytes total — read u16 num_spans=0, then advance 8 bytes.
    // Non-empty: stride is (num_spans + 2) * 4 bytes.
    u16   num_spans;
    u32   pixel_offset;       // MISALIGNED — at byte offset 2 within the Line, not 4.
                              // Relative to Cell.pixel_data_offset; = "byte offset where this
                              // line's first span's pixels start".
    Span  spans[num_spans];   // immediately after the misaligned u32; unaligned 4-byte records
    u16   pad;                // present for non-empty lines; brings total stride to (n+2)*4 bytes
};

struct Span {
    u16   start_x;            // x coordinate of the first opaque pixel in this span
    u16   length;             // number of opaque pixels in this span
    // Pixels: `length` × u16 RGB565 values, packed contiguously into Cell.pixel_data,
    // starting at the line's pixel_offset and consumed in span order.
};
```

The misalignment of `pixel_offset` is intentional: the engine's
post-decode fixup at `div.exe:FUN_00558290` writes back to
`*(int *)((int)piVar2 + 2)`, converting the relative offset to an
absolute pointer. Encoders must match this layout exactly.

### Flags routing

The post-decode fix-up dispatch is at `div.exe:FUN_004e98d0`:

| `flags & 0x10` | `flags & 0x01` | `flags & 0x04` | Path | Status |
|---|---|---|---|---|
| set | — | — | special / non-pixel asset | not reversed |
| clear | clear | — | `FUN_00559310(payload)` — half-dim of 4096 RGB565 px (raw 64×64 cell) | not reversed |
| clear | set   | clear | `FUN_00558290` + `FUN_005592f0(payload)` — span-table sprite, **this format** | ✅ reversed |
| clear | set   | set   | `FUN_00558380(payload)` — span-table sprite with `pixel_offset / 2` (palette / u16-indexed pixels) | partial — same span structure, pixels are 16-bit indices not RGB565 |

## Loader citations

```text
div.exe:0x004e9b80   FUN_004e9b80   imagelist ctor — opens 16 (TIndexedFile, sub-image-cache) pairs
div.exe:0x004e9fe0   FUN_004e9fe0   TIndexedFile ctor — paired blob/index handle
div.exe:0x004e98d0   FUN_004e98d0   per-cell read callback — slurps + post-decode dispatch
div.exe:0x004e9530   FUN_004e9530   reads index entry, fetches LZO blob, calls 0x005a4ce0 decompressor
div.exe:0x005a4ce0   FUN_005a4ce0   LZO1X-1 decompressor (same one as automap)
```

Source path leak: `.\MANAGERS\Imageman.cpp`.

## Validation

Tested via `pkg/assets/cpacked` and `cmd/cell-export` against shipped
`CPackedb.0c` + `CPackedi.0c`:

- All 7208 entries LZO-decompress cleanly.
- 50+ standard-flag cells decode end-to-end into RGB565 (verified
  in tests).
- Spot-checks of cells 0, 5, 500, 1000, 2000, 3000 produce
  recognizable sprites: a bush, a tree, a stone archway, a rocky
  outcrop, a stone wall, and a potion bottle.

The first five entries:

| Entry | Width × Height | LZO size | Uncompressed |
|------:|:--------------:|---------:|-------------:|
| 0 | 170 × 127 | 18 483 | 26 356 |
| 1 | 157 × 104 | 15 437 | 23 750 |
| 2 | 114 × 100 | 11 064 | 15 828 |
| 3 | 131 × 109 | 11 848 | 15 726 |
| 4 | 117 × 102 | 11 721 | 16 302 |

`uncompressed << width * height * 2`, confirming the decompressed
payload is a sparse / RLE sprite, not a raw RGB565 raster.
