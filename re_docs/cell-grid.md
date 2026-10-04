# The world cell grid

<!-- re-derived: blind, against div.exe and the shipped data. -->

One flat allocation of **16,793,600 bytes**: 2,099,200 cells of **8
bytes**, **2048 rows × 1025 columns**. 32 world units per cell, 64 per
block; a block is 2×2 cells.

This is the array the movement path is built on: the walkability grid
rasterizes cubes into these cells and movers test them by flag mask
([`formats/collide.md`](formats/collide.md)), and the per-frame elevation
lookup reads the cell's `+7` byte ([`render-trace.md`](render-trace.md)).
The worldmap global `[0x74eca0]` holds the data pointer at `[+8]`, which
is why that lookup indexes `base + 7 + idx*8` and gates on
`idx < 0x200801`.

## The row pitch is 1025, not 1024

The code computes `row*1024 + col` — but its `col` argument is already
`(X+Y)/32`, so the effective pitch is 1025.

> **`base[y*1024 + x]` reproduces the `shl reg,0xa` you see in the
> disassembly and is wrong.** Every row shifts one cell left relative to
> the previous one: the map shears progressively, and by row 2047 you are
> 2047 cells off. The unambiguous form is
> `base[(worldY/32)*1025 + (worldX/32)]`.

> **Two coordinate conventions coexist in the public API.** Six entry
> points take `(col, row)` with `col = (X+Y)/32`; three take raw
> `(X, Y)`. Normalising them all to `(x, y)` means remembering to add `y`
> back before calling anything that wants `col`.

> **Every coordinate division truncates toward zero, not floors** — the
> `cdq; and edx,mask; add; sar` idiom. `>> 5` on a negative world
> coordinate lands in a different cell, and negatives do reach these
> functions: one entry point explicitly clamps them to 0.

## Two off-by-one hazards worth reproducing deliberately

> **The index guard is `index <= 2099200` — one past the last cell.**
> Feeding coordinates that produce exactly 2,099,200 reads or writes 8
> bytes past the end of the buffer. Using `< count` is *safer* but not
> bit-identical; using `<= count` against a tightly-sized array corrupts
> the heap.

> **`GetFlags` returns `0x1B` for an out-of-range index, not 0** — bits
> 0, 1, 3 and 4, i.e. "blocked and occupied". Out-of-bounds reads as
> solid, which is what keeps things inside the world.

## A cell

```text
+0x00 u16   pathfinder scratch: (searchStamp << 4) | stateBits[0..3]
+0x02 u16   flags -- bits 0..11 OR-able, bits 12..15 a blocker refcount
+0x04 u16   index of the A* node currently attached
+0x06 u8    object / walk-on surface height
+0x07 u8    terrain height
```

| bit | meaning |
|---|---|
| 0 | hard block — **reference-counted** |
| 1 | second block class — also refcounted |
| 2 | an extra block class, tested only by some movement code |
| 3 | set from the owning object record's flag word |
| 4 | dynamic agent occupancy — written directly, **no refcount** |
| 5, 6 | written by one entry point only |
| 7, 8 | from object flags |
| 9 | **the only flag persisted through a save** |
| 10 | this cell has a height that must be step-checked |

The movement path names the same bits by their mask value: bit 0 is
`0x1` (static), bit 1 `0x2` (player-block), bit 2 `0x4` (closed door),
bit 3 `0x8` (no-look-through), bit 4 `0x10` (agent occupancy), bit 7
`0x80` (light), bit 8 `0x100` (lever), bit 9 `0x200` (the map's own
saved blocker) and bit 10 `0x400` (walk-on, climb-gated)
([`formats/collide.md`](formats/collide.md)).

> **The blocker refcount in bits 12..15 has no saturation.** The
> increment is a plain `add 0x1000`, so it carries out of bit 15 and is
> lost. **Sixteen overlapping blockers on one cell wrap the count to
> zero** while the flag bits stay set — and then the first object to
> leave clears a cell that fifteen others still occupy.

## `height.x<n>`

Exactly `{ u8 terrainHeightDiv4; u16 flags; }` per cell, in linear order.
`3 × 2,099,200 = 6,297,600` — **the exact size of all five shipped
files**.

> **The loader throws almost all of it away.** It restores the height
> byte and then reads exactly *one bit* of the saved flags word:
>
> ```text
> cell[7] = fgetc()
> if (cell[7] != 0) cell.flags |= 0x400
> saved = fread2()
> if (saved & 0x200) { cell.flags |= 0x201; cell.flags &= ~0x400; cell.refcount++ }
> ```
>
> Every other saved flag bit is discarded and rebuilt from the object
> records. A port that restores the full word starts with flags the
> original never has.

The two derivations are load-bearing in opposite directions. The climb
bit comes from the *height byte*, not from the file's own `0x400` — that
saved bit is a stale editor-runtime value, and the two disagree on
thousands of cells in every partition: in `height.x0`, 5,537 cells carry
a saved `0x400` with no height at all and 15,842 carry a height with no
saved `0x400`; in `height.x4` it is 294,514 raised cells against 8,879
saved bits. The saved `0x200` outranks the climb bit: a cell the map
blocks keeps no climb bit and takes a blocker refcount of its own, which
is what keeps it blocked when an object standing there is unstamped.

## Pathfinding scratch

The A* node allocator is a **bump allocator capped at 32,767 entries**,
and the 12-bit search stamp **wraps at `0x1000`** — on wrap the pathfinder
clears the scratch words rather than resetting a counter.

Neighbour offsets come from a 16-entry table of `(dcol, drow)` int32
pairs, with a 256-byte reverse table built at construction.

## The query layer over this array

This page also carries the cell-manager citations: `fcn.00571fa0` is the
constructor (`.\WORLD\Cell.cpp`, loading-screen status "Loading cell
manager"), `fcn.005706d0` the coordinate→cell helper (clamps to grid
bounds), `fcn.005709e0` an area query that accumulates into the scratch
list at `0x74cca0` (built with `fcn.0056fd50` add / `fcn.0056fe20`
clear), and `fcn.005719f0` a directional variant over the quadrant table
at `0x74cba0`. The constructor's other allocation is a node/record pool
of `0xdffe4` bytes.

> **The earlier reading of this page — a `1024 × 1024` grid of 16-byte
> cells, each holding the head of a per-cell bucket list of the objects
> in it — is superseded by the cell record above.** It conflicts with the
> elevation lookup, which indexes this global at `idx*8` under a
> `0x200801` gate ([`render-trace.md`](render-trace.md)), and with the
> walkability grid, which reads flags at `+2` and the two height bytes at
> `+6`/`+7` ([`formats/collide.md`](formats/collide.md)). An 8-byte cell
> has no room for a bucket-list head, so what these queries gather is NOT
> reconciled — only the citations above are kept.

## Status

- Cell size, grid dimensions and the 1025 pitch ✅.
- The two coordinate conventions and the truncating divisions ✅.
- The one-past-the-end index guard and the `0x1B` out-of-range flags ✅.
- All eleven flag bits and the refcount field ✅.
- The unsaturated refcount ✅.
- `height.x<n>`'s layout ✅ — `3 × cells` is the exact file size for all
  five maps.
- The loader's one-bit restore ✅.
- The A* scratch, its cap and its stamp wrap ✅.
- The query layer's role 🟡 — the citations above are kept, the
  bucket-list reading of them is retracted and nothing replaces it.
- Bits 5 and 6's meaning 🟡 — their sole writer is identified; what
  distinguishes them is not.
- The 16-entry neighbour table's ordering convention 🟡 — the table and
  its reverse map are read; which index is which direction is not pinned.
