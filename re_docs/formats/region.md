# `global\region.00N` — named region tables (per subsystem)

The world's **named regions** are labelled polygon areas the engine fires "enter region" triggers against.
Region names thread through the scripting layer — e.g. trap binding rejects an unknown region with `"Unknown region %s in region trigger trap"` ([`../traps.md`](../traps.md)), and Osiris quest logic keys off the same names.

The eight `region.00N` files are **not** world partitions — each is the region table for a **different subsystem**, identified by its version string.
Verified across all eight, the `"Divinity Regions"` magic, version, count and named-record framing hold for every one:

| File | Version | Count | Subsystem |
|---|---|---:|---|
| region.000 | `StoryV1.0` | 435 | Osiris story / quest trigger areas |
| region.001 | `MusicV1.0` | 155 | music zones |
| region.002 | `MonsterV1.0` | 111 | monster-spawn regions ([`../monsters.md`](../monsters.md)) |
| region.003 | `TreasureV1.0` | 1 | treasure region |
| region.004 | `ReverbV1.0` | 89 | audio reverb zones ([`../sound-runtime.md`](../sound-runtime.md)) |
| region.005 | `DungeonV1.0` | 90 | dungeon areas |
| region.006 | `TrapsV1.0` | 81 | trap trigger regions ([`../traps.md`](../traps.md)) |
| region.007 | `ShroudV1.0` | 90 | shroud / fog-of-war regions |

The named regions a trap or quest references live in the matching subsystem file (a trap region in `region.006`, a monster region in `region.002`, …).
Each region record carries its own **world ID**.

The loader `FUN_0058e690` confirms the subsystem distinction from the binary side: it validates the file's version against a requested **region type**, erroring with `"Regiontype desired does not match <story|music|…>"`.
The full type enum (from that validation chain):

```text
0 story   1 music   2 reverb   3 treasure   4 monster
5 teleport   6 dungeon   7 traps   8 shroud
```

The file *number* does not equal the type enum (e.g. `region.002` is the **monster** table = type 4), and **teleport** (type 5) has no shipped `region.00N` file in this install.

## Layout source

The record and vertex layouts below come from [Raan's region-format findings](https://github.com/vvoland/opendivine/issues/10#issuecomment-6711735678), including a C# `BinaryReader` parsing example and a rendered-region screenshot.
They supersede the earlier interpretation of the geometry as cell-relative 32-bit floats interleaved with baked pointers.
The reported layout now walks all eight shipped files to EOF with their declared record counts.

All integers and floating-point values are little-endian.
String lengths are byte counts; the existing file inspection identifies trailing NULs in the subsystem tag and region names.
Raan describes the text encoding as UTF-8.

## Header

Offsets are from the start of the file; `L` is the subsystem/version string length.

```text
+0        byte magic[21]       "Divinity RegionsV1.0\0"
+21       u32  sub_tag_len     L
+25       char sub_tag[L]      "<Subsys>V1.0\0" (Story / Music / Monster /
                              Treasure / Reverb / Dungeon / Traps / Shroud)
+25+L     u32  count           number of region records
+29+L     Region records[count]
```

**Records start immediately after `count`.**
There is no extra pair of file-level pointers or coordinate values between the count and the first record; those bytes belong to the first record's fields.

## Region records

Offsets are relative to the start of each record; `N` is the region name length and `V` is the vertex count.

```text
+0        u32  var0            unknown custom variable
+4        u32  var1            unknown custom variable
+8        u32  var2            unknown custom variable
+12       u32  world_id        world number for this region
+16       u32  name_len        N
+20       char name[N]         region name (may be empty)
+20+N     u8   flag            region flag; meaning unknown
+21+N     u32  vertex_count    V
+25+N     Vertex vertices[V]   24 bytes per vertex
```

Each record occupies **`25 + N + 24 × V` bytes**, with no alignment padding after the one-byte flag.
Read exactly `count` records sequentially; the next record starts immediately after the current vertex array.
The names `var0`, `var1` and `var2` are placeholders, not established semantics.

The shipped `region.000` declares **435** regions.
Earlier heuristic scanning recovered 431 names; that was not a complete record walk and does not change the declared count.
Sample names show the role clearly:

```text
front_house_Mardaneus  george_attack  Aleroth_orb_pentagram_{West,north,East}
house_lanilor  herb_garden_Lanilor  pool_aleroth  dh_house4 … dh_house9
```

These are building interiors, quest set-pieces (the Aleroth pentagram orbs), and town-house clusters: the areas story scripts and traps need to name.
The single Treasure region is unnamed.

## Vertex encoding

Each vertex has a fixed **24-byte** stride:

```text
+0        f64  x               absolute global-map X pixel coordinate
+8        f64  y               absolute global-map Y pixel coordinate
+16       byte opaque[8]       internal engine data; meaning unknown
```

Coordinates are **IEEE 754 64-bit doubles**, not 32-bit floats or integers, and are **absolute pixel positions**, not cell-relative coordinates.
Reading either component as four bytes misaligns the stream and produces distorted polygons (Raan reports wave/spiral artifacts).
The earlier apparent `float` values and recurring `64` words were not evidence of a cell-relative vertex format.

A geometry-only reader can skip the final eight bytes of each vertex, but must still consume the full 24-byte stride.
Preserve those bytes unchanged when round-tripping files; their interpretation as padding, reference flags or other internal data is not established.
No pointer reconstruction is needed to walk the reported on-disk layout.

## Runtime containment (point-in-region)

The **runtime test is a plain point-in-polygon** over each region's vertex loop.
The entry point `FUN_004ff040(x, y)` (two `double`s) wraps the core **`fcn.004fef20`**, a double-precision **winding-number** test: it walks the region's edge list and, per edge, computes the edge's x-crossing at the test point's `y` (`(edi.x-edx.x)/(edi.y-edx.y) * (edx.y - y)` reversed to an x) and compares it to the point's `x`, accumulating a signed crossing count in `ebp`; the point is **inside when the winding magnitude reaches `±4`** (`cmp bp,4` / `cmp bp,0xfffc` — one full turn).
The per-edge `cmp eax,6; ja` selects the crossing case by edge orientation.

The **point→region lookup `FUN_0058d620`** walks the region list (count at `mgr+0x14`) and returns the first region whose polygon contains the point (via the per-region tester `fcn.004f5a00` → `fcn.004fef20`).

This winding point-in-polygon test backs the **shroud region-flood reveal** (`FUN_0053f780` → this test, [`shroud.md`](shroud.md)), the **Osiris region enter/leave trigger areas**, the per-region **music/reverb** (`region.004`, [`../sound-runtime.md`](../sound-runtime.md)), and **no-magic zones** — all "which named region is this point in?" queries.

## Loader observations

`FUN_0058e690` parses sequentially via a chain of `fread(buf, size, 1, f)` calls (observed chunk sizes `4, 8, 12, 16, 24, 28`) over a count-bounded loop.
It verifies the header tag with a byte-by-byte compare after a `fread(…, 0x15)` and loops regions against a count (`cmp [esi+0x24], …`).
It also constructs several empty list members per region (the `esi+0xc..0x30` slots zeroed via `FUN_004fa540`).
These runtime containers do not imply that the disk format is a heap dump or requires pointer fixups to recover the vertex stream.

## Status

- Subsystem indexing ✅ — per-subsystem region tables, not world partitions; verified across all eight files and confirmed by the loader's nine-value region-type enum.
- Role ✅ — named trigger areas referenced by traps, monsters, audio, Osiris and quests, each in its matching subsystem file.
- Runtime containment ✅ — double-precision winding-number test over a region's polygon.
- Record framing and vertex stride ✅ — all eight shipped tables walk to EOF with their declared counts using four leading `u32` fields, a length-prefixed name, one-byte flag, vertex count and fixed 24-byte vertices; the absolute `f64` pixel-coordinate interpretation follows Raan's findings.
- Unknown fields 🟡 — meanings of `var0`, `var1`, `var2`, the region flag and the final eight bytes per vertex remain unresolved.
- **Reimplementation note** — parse the sequential records and double-precision vertex loops directly, retaining the world ID and opaque fields rather than reconstructing alleged baked pointers or deriving regions from another source.
