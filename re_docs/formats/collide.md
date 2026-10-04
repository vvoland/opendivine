# `static\imagelists\Collide.<n>` — per-sprite collision/cube tables

Flat arrays of 16-byte records. The shipped install carries 13 files
(Collide.0 .. Collide.12; Collide.2 is empty). Each entry is the
collision / 3D cube data for the matching sprite in `CPacked.<n>`,
loaded by the engine from `.\CUBE\Cube.cpp`.

## Layout

```text
struct CollideFile {
    Record records[file_size / 16];
};

struct Record {
    i16   anchor_x;   // [0] cube anchor X: initial sprite-relative offset in file; overwritten
                      //     at runtime with the object's world X (FUN_00448020: *psVar6 = object_x)
    i16   anchor_y;   // [1] cube anchor Y: initial sprite-relative offset in file; overwritten
                      //     at runtime with the object's world Y (FUN_00448020: psVar6[1] = object_y)
    i16   rt_timer;   // [2] RUNTIME only — always 0 in file; written by FUN_004eca40 to same
                      //     timer value as i16[10] (psVar5[2] = sVar4)
    i16   x_extent;   // [3] asymmetric rightward reach (right-edge offset from anchor); vs width[5]
                      //     which is the symmetric box width. FUN_00448020 AI centre-X =
                      //     avg(anchor_x + width/2, anchor_x + x_extent)
    i16   z_height;   // [4] vertical Z extent of the 3D cube above ground (tiles/units);
                      //     range 1–175 in shipped data. Confirmed consumers (via the cube
                      //     accessor FUN_00414e70 = cube_base + 22*index): FUN_00415740 uses
                      //     z_height/2 + base, FUN_00415dc0 uses z_height + base — the cube's
                      //     mid- and top-height reference points (no-cube fallback = base + 64)
    i16   width;      // [5] cube collision width; halved for screen-centre calc:
                      //     FUN_004eca40: centre_x = anchor_x + width/2 + camera_scroll
    i16   type;       // [6] cube type: 0=no collision, 1=static obstacle (in file),
                      //     2=activated/interactive; written 2 by FUN_0043b2b0 (psVar8[6]=2)
    i16   flags;      // [7] runtime flags: 0 in file; bit0 set to 1 by FUN_00471c00 when
                      //     merged-array slot is claimed; |= 0x10 at update time (FUN_004eca40)
};
```

Total file size = `count × 16` bytes (no header).

## Pairing with CPacked imagelists

Most Collide files have an entry count that matches the corresponding
CPacked imagelist exactly:

| File         | size       | records | CPacked.<n> entries | match |
|--------------|-----------:|--------:|--------------------:|:-----:|
| `Collide.0`  |    115,328 |  7,208  |              7,208  |   ✓   |
| `collide.1`  |  1,261,648 | 78,853  |             78,853  |   ✓   |
| `COLLIDE.2`  |          0 |      0  |              3,363  |  (empty — floor tiles need no cube data) |
| `COLLIDE.3`  |     21,376 |  1,336  |              1,336  |   ✓   |
| `Collide.4`  |      6,128 |    383  |                383  |   ✓   |
| `COLLIDE.5`  |     11,744 |    734  |                262  |   ≠   |
| `Collide.6`  |        880 |     55  |                 50  |   ≈   |
| `Collide.7`  |     77,408 |  4,838  |              4,838  |   ✓   |
| `Collide.8`  |     10,672 |    667  |                663  |   ≈   |
| `Collide.9`  |        144 |      9  |                  9  |   ✓   |
| `Collide.10` |  1,252,256 | 78,266  |                278  |   ≠   |
| `Collide.11` |      2,096 |    131  |                131  |   ✓   |
| `Collide.12` |     22,192 |  1,387  |              1,387  |   ✓   |

Where the counts diverge:
- `Collide.10` (78 k entries vs CPacked.10's 278) likely indexes into
  CPacked.1's animation frames (which has 78,853 entries — close).
- `COLLIDE.5` and `Collide.6` are slightly larger; possibly because
  some CPacked.5/6 sprites have multiple cube entries.

(Verified by [`pkg/assets/collide`](../../pkg/assets/collide).)

## Hex dump — `Collide.0` first 16 bytes

```text
00000000  12 00 6a 00                                 max_x = 18, max_y = 106
00000004  00 00 6f 00                                 tail[0..3]
00000008  3a 00 3d 00                                 tail[4..7]
0000000c  01 00 00 00                                 tail[8..11]
```

## Loader citation

```text
div.exe:0x004717e0   FUN_004717e0   loader entry — fopen("static\\imagelists\\collide.%d", ...);
                                    fread(record, 0x10, 1, fp) repeated to count
                                    Source path leak: ".\\CUBE\\Cube.cpp"
```

## In-memory representation

On load, records expand to 22 bytes (11 × i16) in the merged "all-cubes"
array (`FUN_004717e0`: `FUN_004ec460(total_count, 0x16, 1)`). Three
runtime-only fields are appended at i16[8..10]:

| In-memory index | Source | Notes |
|---|---|---|
| 0..7 | from file | on-disk fields above; [0] and [1] overwritten with world position at runtime |
| 8 | runtime | screen centre_x = `anchor_x + width/2 + camera_scroll` (`FUN_004eca40`) |
| 9 | runtime | screen centre_y — time-based Y value (`FUN_005e5d40`) |
| 10 | runtime | same timer value as i16[2] (`FUN_004eca40: psVar5[10] = sVar4`) |

Slot allocation: `FUN_00471c00` copies the 16-byte file record into the first 16 bytes
of a free 22-byte merged slot, then sets bit 0 of byte 14 (= i16[7].low) to mark it
"in use". Multiple sprites sharing the same cube type each receive their own slot.

The merged array is accessed as `(int*)*DAT_006592dc`, where
`(*DAT_006592dc)[0]` = data base, `[1]` = count, `[2]` = stride (22).

## Loader citations

```text
div.exe:0x004717e0   FUN_004717e0   loader; opens collide.%d; count = filesize>>4; stride 0x10
                                    Source path: ".\\CUBE\\Cube.cpp"
div.exe:0x004eca40   FUN_004eca40   runtime cube updater: writes i16[8,9,10]; flags i16[7] |= 10
div.exe:0x0043b2b0   FUN_0043b2b0   animation-cube linker: writes i16[6]=2, adjusts i16[0,1]
div.exe:0x00414e70   FUN_00414e70   cube-element accessor: returns cube_base + 22*index (stride from base+8)
div.exe:0x00415740   FUN_00415740   reads i16[4] z_height → z_height/2 + base (mid-height ref); fallback base+64
div.exe:0x00415dc0   FUN_00415dc0   reads i16[4] z_height → z_height + base (top-height ref); fallback base+64
```

## Runtime collision — rasterized cell flags (RESOLVED; supersedes the earlier pipeline)

**The earlier "agent move → `FUN_00415120` → `FUN_00571df0` cube
cell-query → sqrt vs width/x_extent" pipeline is retracted** — those
functions are the *combat/interaction reach* test (below), not
movement. Movement blocking is entirely **cell-grid based**: a cube is
rasterized into per-cell flag bits at object placement, and movers test
whole cells. There is **no sqrt, no circle, no per-move geometric test
anywhere in movement.**

### The walkability grid (worldmap `[0x74eca0]`, `[+8]` = base)

8-byte cells, index `v·1024 + u` where **`u = (x+y)>>5`, `v = y>>5`**
(signed `/32`; identical iso mapping at every site); valid gate
`index ≤ 0x200800`. Per cell: flags `u16` at `+2`, height bytes at `+6`
(object add-on, quarter-units) and `+7` (base). Primitives:
`fcn.00419530` SetCellFlags (`or [cell+2], mask`), `fcn.00414ef0`
ClearCellFlags, `fcn.00571df0` **CellLineBlocked** — a two-octant
Bresenham walk testing `flags & mask` per cell (a line-of-cells
raycast, *not* a cube gatherer).

### Rasterization — `fcn.00572100` AddObjectToWorld → `fcn.0056d720`

On placement (map loader `0x59ce90`, object-manager sites `0x585…`,
`0x589…`, `0x5a2…`), the cube stamps an **inclusive cell rectangle**:

```text
x = objX + anchor_x(i16[0]);  y = objY + anchor_y(i16[1])
u ∈ [ (x+y)>>5 , (x+y+x_extent)>>5 ]      // i16[3] spans the iso u axis
v ∈ [ (y−width/2)>>5 , y>>5 ]             // i16[5]/2 spans the v (y) axis
per cell: flags |= mask
          if mask & 3: blocker refcount (top nibble) += 1     // 0x56d7fd
          if mask & 0x400 && (h − 4·byte[+7]) > 4·byte[+6]:
              byte[+6] = (h − 4·byte[+7] + 3) >> 2            // climb height
```

with `h = z_height(i16[4]) + baseZ`. `fcn.0056e2c0` → `fcn.0056d890`
is the exact inverse (refcount −1; bits clear when it reaches zero).
`x_extent` and `width` are never compared, averaged, or maxed — they
span the two axes of the rectangle. The `FUN_00448020` centre blend is
AI-only.

**The mask comes from the object flags word, NOT the cube `type`**
(i16[6] is unread in the movement path — retraction of the earlier
"type gates blocking"):

```text
if sb_player_block: mask = sb_walk_through ? 0 : 0x2
elif sb_door:       mask = (sb_closed ? 0x4 : 0) | (sb_locked ? 0x1 : 0)
else:               mask = sb_walk_through ? 0 : 0x1
plus: sb_light → |0x80 · sb_lever → |0x100 · catalogue sb_walk_on → |0x400
      catalogue bit23 → |0x800 · sb_no_look_through → |0x8
      if mask & 1: mask &= ~0x400          (hard blockers don't climb)
```

**Placement gate — mask/height, never type** (`fcn.00572100`
`0x5721ec`): after deriving the mask, `fcn.00572100` calls the
rasterizer `iff (mask ≠ 0) || (z_height + baseZ ≠ 0)` — `test ax,ax /
jne` then `test ebp,ebp / je skip`, where `ebp = z_height(i16[4]) +
baseZ`. The cube `type` field (i16[6]) is **not tested anywhere on the
path to the stamp**. So a solid wall whose collide record has `type=0`
(common — e.g. the region-0 start-room stone walls id 4220/4221/4224,
`type=0 z=148 w=127`, deriving mask `0x009` = static|no_look_through)
**still blocks**. A reimplementation must gate the stamp on the derived
mask, not on `type`; gating on `type ≠ 0` silently drops roughly half
the walls (24,921 of 57,320 stamped cubes in `world.x0` are type-0 hard
blockers) and lets the player walk through them.

One more blocker source is not an object at all. `height.x<n>`'s saved
bit 9 (`0x200`) hard-blocks its cell outright — `flags |= 0x201`, the
climb bit cleared again, and a blocker refcount of its own — which is
how a map walls off cells that no object record covers
([`../render-trace.md`](../render-trace.md), the loader). It is a
refcounted blocker like any other, so unstamping an object that shares
the cell leaves the map's own block standing.

Door/lever state changes **re-run remove+add** — that is how "locked
doors block" works. The map loader also sets bit `0x1` directly for
cells whose record `word[+4] ≠ 0` (`0x59cfa0`).

### Movement — leg-based steppers; blocked = leg cancelled

The per-tick movers never test geometry:

```text
behaviour tick → stepper (0x40f0a0 / 0x4433xx / 0x41a670): try 16 iso dirs
    accept dir iff octagonal-distance score improves (0x40ecb0)
        AND walkability 0x56f3c0(worldmap, cell, dir, mask 0x13) passes
    commit: facing; 0x427630 (CellDx/Dy = float table 0x654f50[dir]·Walkspeed,
            Walkcount = f2[dir]·(32/speed)); 0x4270a0 (occupancy bit 0x10
            old→new leg-destination cell)
    no dir passes: virtual +0x14 (stop) — the move is CANCELLED; no
            axis-separation, no clamp, no geometric slide ("sliding"
            emerges from the 16-direction greedy re-aim)
per frame → 0x427d30: Fx(+0xc) += CellDx(+0x22c); Fy += CellDy;
    X/Y = ftol(Fx/Fy); Walkcount(+0x278)-- ; no collision re-check mid-leg
```

`fcn.0056f3c0` walkability: bounds check, dest cell = cell +
`DIR16[dir]` (table `0x654e50`, `{i32 du, i32 dv}`×16), `flags & mask`
→ blocked, plus a per-direction corner/pass-through cell chain and the
**climb gate** on `0x400` cells: `byte[n+6]+byte[n+7] − byte[cur+6] >
0x50` (80 units) → blocked. Movers test with **mask `0x13`** (static
`0x1` | player-block `0x2` | other-agent occupancy `0x10`) and
contribute **no radius** — an agent occupies exactly one cell (its
bit-`0x10` leg destination, `0x4270a0`, skipped when
`+0x220 & 0x180`).

**The movement data tables (dumped).** The 16 iso directions are
0 = N (`(du,dv)=(-1,-1)`) rotating clockwise; the three tables share the
index:

```text
dir  DIR16 (du,dv)   CellDx/Dy/factor        walkability corner cells (all
0x654e50            0x654f50 (screen vel)     climb-gated on 0x400)
 0   (-1,-1)   ( 0.0,-1.0)·1   →Δ(-1,-1),Δ(-1,0)
 1   (-3,-2)   (-0.5,-1.0)·2   →Δ(-2,-2),(-2,-1),(-1,-1),(-1,0),(-2,0),(0,-1)
 2   (-2,-1)   (-1.0,-1.0)·1   →Δ(-1,-1),Δ(-1,0)
 3   (-3,-1)   (-1.0,-0.5)·2   →Δ(-2,-1),(-1,-1),(-1,0)
 4   (-1, 0)   (-1.0, 0.0)·1   →dest only
 5   (-1, 1)   (-1.0, 0.5)·2   →Δ(-1,0),Δ(0,1)
 6   ( 0, 1)   (-1.0, 1.0)·1   →dest only
 7   ( 1, 2)   (-0.5, 1.0)·2   →Δ(1,1),Δ(0,1),Δ(1,0)
 8   ( 1, 1)   ( 0.0, 1.0)·1   →Δ(0,1),Δ(1,0)
 9   ( 3, 2)   ( 0.5, 1.0)·2   →Δ(2,2),(2,1),(2,0),(1,0),(1,1)
10   ( 2, 1)   ( 1.0, 1.0)·1   →Δ(1,1),Δ(0,1),Δ(1,0)
11   ( 3, 1)   ( 1.0, 0.5)·2   →Δ(2,1),(1,1),(1,0),(2,0),(3,0),(0,1)
12   ( 1, 0)   ( 1.0, 0.0)·1   →dest only
13   ( 1,-1)   ( 1.0,-0.5)·2   →dest only
14   ( 0,-1)   ( 1.0,-1.0)·1   →dest only
15   (-1,-2)   ( 0.5,-1.0)·2   →Δ(-1,-1),Δ(0,-1)
```

`0x654f50`'s third float is the **Walkcount multiplier** (`1` for the 8
primary facings, `2` for the 8 half-step intermediates — finer diagonals
at the same net speed; `Walkcount = factor·(32/speed)`). The
**octagonal distance** `fcn.0040ecb0` (used as the stepper's greedy
score and the A* heuristic) is `dist = 1.0390625·max(|dx|,|dy|) +
0.3984375·min(|dx|,|dy|)` (shift-add of `max>>{0,5,7}` + `min>>{2,3,6,7}`).
*(Note `0x654ed0` — sometimes near these tables — is a doubled `0..15`
wrap table for `dir ± n mod 16`, not a facing LUT; sprite facing derives
straight from the direction index.)*

### `FUN_00415120` / `FUN_00440320` — combat/interaction reach (not movement)

Virtual slot 1 of the fight controllers (`CAgentFight` `0x609020` /
`CClientFight` `0x60c2c8`; `CPartyFight` `0x60cbb4` overrides with
`0x440320`). Returns bool "target within touch range": first a
`CellLineBlocked(A, B, mask 0x1)` occlusion test, then — when both
agents have cubes — the **min distance over 16 pairs of 4 cube corner
points** (built from the runtime centre i16[8]/[9], `width`,
`x_extent`) compared against `size_byte([agent+0x228]+0x104) · 32`
(`CPartyFight`: `· 64`); else a cell-adjacency fallback (|Δu|,|Δv| ≤ 1,
extended for size ≥ 2). This is where the sqrt lives — reach, not
blocking.

## Status

- Runtime resolution ✅ **(RESOLVED — full rewrite above)** — movement
  blocking is rasterized cell flags (`0x572100`→`0x56d720` stamp,
  `0x56f3c0` walkability, mask from the object flags word), with the
  leg-based stepper chain (`0x40f0a0`→`0x427630`/`0x4270a0`→`0x427d30`)
  cancelling blocked legs; the old sqrt pipeline reclassified as the
  fight-controller reach test. The former 🟡 is closed.
- File-level layout ✅ — fixed 16-byte stride; counts validated against shipped CPacked imagelists.
- i16[0] = anchor_x 🟡 — initial sprite-relative offset in file; overwritten with world X at runtime.
- i16[1] = anchor_y 🟡 — same; overwritten with world Y at runtime.
- i16[2] = rt_timer ✅ — RUNTIME only; 0 in file; confirmed from `FUN_004eca40`.
- i16[3] = x_extent ✅ — **disambiguated vs `width` (i16[5]) in `FUN_00448020`**:
  the AI centre-X is `avg(anchor + width/2, anchor + x_extent)`, where `width`
  (i16[5]) is read **symmetrically** (`anchor + width/2`, the `sar …,1` halving
  at `0x448063`) and `x_extent` (i16[3], `movsx …,word[esi+6]` at `0x4480cb`)
  is read as `anchor + x_extent` — the **right-edge position**. So `width` is
  the cube's *symmetric* collision-box width (half each side of the anchor) and
  `x_extent` is the *asymmetric rightward reach* (the sprite's right-edge offset
  from the anchor, which differs from `width/2` for off-centre sprites); the AI
  reference point blends the two.
- i16[4] = z_height ✅ — vertical Z extent; **confirmed consumers found**:
  via the cube-element accessor `FUN_00414e70` (`cube_base + 22·index`),
  `FUN_00415740` reads it and computes `z_height/2 + base[edi+0x14]` (the
  cube's **mid-height** reference) and `FUN_00415dc0` computes `z_height +
  base` (the **top-height** reference); both fall back to `base + 64` when
  the object has no cube (index `-1`). So z_height is the cube's vertical
  reach, consumed by the agent proximity/aim cluster (`0x415…`) to derive a
  vertical reference point (half-height for the centre, full for the top).
- i16[5] = width ✅ — collision cube width; halved for screen centre calc (`FUN_004eca40`).
- i16[6] = type ✅ — 0/1/2 in file; set to 2 at runtime by `FUN_0043b2b0`.
  **Not read by the movement path** — blocking masks derive from the
  object flags word (retraction of "type gates collision").
- i16[7] = flags ✅ — 0 in file; bit0 set by `FUN_00471c00`; |= 0x10 by `FUN_004eca40`.
