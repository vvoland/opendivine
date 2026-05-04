# `div.exe` per-frame render trace

In-progress reverse engineering of the per-frame rendering pipeline.

## Render entry points

`div.exe:0x004ab4a0` — main per-frame render (`render.cpp:0x5c1`).
Calls a chain of subsystems including:

- `0x004cdb70`  per-layer setup (calls `0x004cd5a0` per object to
  compute an isometric scene-anchor position via `FUN_004f7b40`).
- `0x004cda30`  per-layer handle update.
- `0x004c72b0`  per-layer draw — calls each layer's `vtbl[2]` Render method.
- `0x004f1c30`  late-pass draw of UI overlays (calls `0x004f1b10` per item,
  which forwards to `surface->vtbl[14]`).

`div.exe:0x004ab270` — alternate render path (used in some modes).
Calls `0x00572270` for object-cell debug overlays and `0x00597740`
for fog/lighting state.

## Floor-tile renderer (FOUND)

`div.exe:0x0059d9d0` — the floor-tile blit.  Iterates a 64-pixel grid
in screen space and per cell:

1. Indexes a **QuickObject cache** (`WorldManager[3]`, 0xdffe4 bytes)
   with stride `0x88` (136 bytes / cell) holding floor_id, overlay_id,
   per-cell flags, and elevation.
2. If the cell's elevation field (`psVar12[6]`) is `0xffff`, falls back
   to `byte_7` of the world spatial-hash bucket (DAT_0074eca0 + 8) × 4.
3. Calls `surface->vtbl[7]` (`*DAT_00746a58 + 0x1c`) with 8 args:
   `(floor_handle, overlay_handle, ?, count, dim, flags, special, ?)`.

The scroll offset is set once per frame via `surface->vtbl[6]` (0x18).

## Object-cube anchor (was misidentified as the world-object sort key)

`div.exe:0x004cd5a0` is **not** the per-object world sprite sort.  It
belongs to `FUN_004cdb70` (per-layer setup at `0x004ab598`) and feeds
the magic / particle / floating-icon subsystems (the FX layer).
Output of `FUN_004f7b40`:

```c
out_X = in_X + in_Y - 65536      // isometric X+Y collapse
out_Y = in_Z                      // elevation only (no XY contribution)
out_Z = (65536 - in_Y) * 2        // depth-sort key for FX
```

This drives FX overlays, not world objects.  The world-object depth
sort lives in `CSpriteSorter::Render` (next section).

## World-object depth sort — FOUND (`CSpriteSorter::Render`)

`div.exe:0x00547000` (`FUN_00547000`), called per-frame from
`FUN_004ab4a0` at `0x004ab56b`.  Three-stage algorithm:

1.  **Back-emit pass** (`0x00547042`-`0x00547080`).  Sprites whose
    sortbox `+0x3c & 0x400` is set are removed from the topo list
    and drawn at z=30000 outside the topo (master-slave 0x100/0x40
    children get z=30001 and z=29999).

    **The 0x400 bit is exclusively for `CGoreAttachedEffect`.**  The
    only writer of `sortbox[+0x3c] |= 0x400` is `FUN_00414400` at
    `div.exe:0x004146c8` — `CGoreAttachedEffect::AddSortbox`,
    identified via vtable `0x00608fd0` → descriptor `0x0062d558` →
    RTTI string `0x0064a07c`.  It fires when a gore animation
    reaches its last frame, sinking the blood/decal to z=30000 so
    it lays on the ground as a permanent splat.

    Sibling classes `CAnimationAttachedEffect` (vtable `0x00608fb0`,
    method `FUN_00414100`) and `CAnimationVerticalEffect` (vtable
    `0x00608fe0`, method `FUN_00414810`) are also attached-effect
    builders but NEVER set 0x400.  Static world props (Tree=156,
    Fence=85, Wall=20, etc.) flow through their own AddSortbox
    classes and likewise never set this bit.

    Static-prop dispatch path (verified): per-frame entry
    `FUN_004ab4a0` → `FUN_004a3fd0` (at `0x004a3fd0`) → `FUN_00422830`
    (at `0x00422830`) iterates the visible cell list at
    `param_1[0]..param_1[1]` and for each object with `+0x218 == 1`
    and a non-null vtable at `+0x2e4`, invokes `vtbl+0xc` — the
    per-class AddToSortBox.  Static props go straight into the
    topological sort without ever touching the back-emit branch.

    OpenDivine note: `cmd/divine/main.go` routes ALL visible
    objects through topo, which matches the engine: every static
    world prop participates in the pairwise depth-sort, just as
    the engine does.  Gore decals are not yet implemented.

2.  **Sort-record build** (`0x005470c0`-`0x00547144`).  For each
    sprite a 7-int record is materialised at `local_6ff8` from the
    `*DAT_006592dc` runtime imagelist (the cube/collide table loaded
    from `static\imagelists\collide` by `FUN_004717e0` at
    `0x004a0928`).  Loader stride is 0x10 on disk, **0x16 in RAM** —
    six trailing bytes (three shorts at +0x10/+0x12/+0x14) are the
    runtime mirror written by `FUN_0059c7e0` at `0x0059c9c2`:

    ```text
    imgRec[+0x00] = AnchorX        // collide.<n> field, sprite-local
    imgRec[+0x02] = AnchorY        // collide.<n>
    imgRec[+0x04] = (mutated to entity Z by FUN_0059c7e0)
    imgRec[+0x06] = XExtent        // collide.<n>
    imgRec[+0x08] = ZHeight        // collide.<n>
    imgRec[+0x0a] = Width          // collide.<n>  (cube Y-axis extent)
    imgRec[+0x0c] = Type           // collide.<n>
    imgRec[+0x0e] = Flags          // collide.<n>
    imgRec[+0x10] = AnchorX + Width/2 + entity.worldX  (runtime mirror)
    imgRec[+0x12] = AnchorY - Width/2 + entity.worldY  (runtime mirror)
    imgRec[+0x14] = entity.layer                       (runtime mirror)
    ```

    Sort-record fields:

    ```text
    r[0] = imgRec[+0x10] + imgRec[+0x12]
         = AnchorX + AnchorY + worldX + worldY    (iso-X+Y depth)
    r[1] = XExtent + r[0]                          (right end of bbox)
    r[2] = imgRec[+0x12]                           (bbox top in iso-Y)
    r[3] = Width/2 + r[2]                          (bbox mid in iso-Y)
    r[4] = imgRec[+0x14]   = entity.layer (Z)
    r[5] = imgRec[+0x08]   = ZHeight   (cube z extent)
    r[6] = local_700c                              (original index)
    ```

    Sprites with `sprite[+0x3c] & 0x200` (override-sort) skip the
    imgRec lookup and read seven ints from `sprite[+0x68..+0x80]`.

    **Field-mapping verified** (re-checked 2026-05-03 against
    `FUN_0059c7e0` at `0x0059c9c2..0x0059c9d4` and the sort-record
    builder at `0x005470c0..0x00547144`): the half-extent baked
    into r[3] and into the runtime mirrors at +0x10/+0x12 is
    `psVar10[5]` — the short at `collide+0x0a`, i.e. **Width**.
    The 2-byte field at `collide+0x04` is the runtime-only
    `RtTimer` slot (always 0 on disk; not used in the sort).
    The current `sortRecord` builder in `cmd/divine/main.go`
    uses `cubeW = collide.Width`, which matches the engine.

3.  **Pairwise compare + topo recursion** (`0x00547180` onward).
    For each ordered pair (i,j) the engine first runs the AABB
    overlap test `FUN_00471690` on `sprite[+0x40..+0x4c]`.  If the
    bboxes overlap, it calls comparator `FUN_00546e40`:

    ```c
    int cmp(a, b) {
        if (b[3] <= a[2]) return +1;          // b above a's top → a in front
        if (a[3] <= b[2]) return -1;          // a above b's top → b in front
        if (a[5] + a[4] < b[4])               // Z stack: a's top ceil < b's floor
            return (b[1] <= a[0]) ? +1 : -1;  // X tiebreak
        if (b[5] + b[4] < a[4])
            return (a[1] <  b[1]) ? +1 : -1;
        return (b[0] <= a[0]) ? +1 : -1;      // X tiebreak
    }
    ```

    The "loser" of the compare gets a dependency edge appended to
    `sprite[+0x90/0x94]`.  The topo-emit pass `FUN_00546ec0`
    (`0x00546ec0`) DFS-walks deps, then calls `vtbl[0]` to blit, with
    a `visited` flag (`sprite[+0xa4]`) preventing cycles and a
    safety abort at 3000 recursive calls.

The whole world-AABB at `sprite[+0x40..+0x4c]` is the screen-space
extent, but the algorithm works in any consistent space; OpenDivine
uses world-space top-left + sprite W/H and gets identical edges.

## Spatial-hash bucket layout

`DAT_0074eca0 + 8` is a 16 MB buffer (`0x1004000` bytes).  Indexed by:

```text
bucket_index = (floor((Y + flags)/32)) * 0x400 + floor(((X+Y)/32))
addr         = base + bucket_index * 8
```

Per-bucket layout (8 bytes):

| Offset | Bytes | Set by                                   | Read by                             |
|-------:|------:|------------------------------------------|-------------------------------------|
| +0..1  | 2 | `FUN_0056d720` ORs `layer & 0xfff` into low 12 bits | `FUN_0059d9d0`, debug overlays |
| +2..3  | 2 | various flag setters                     | many                                |
| +4..5  | 2 | unknown                                  | unknown                             |
| +6     | 1 | `FUN_0056d720`/`FUN_0056dfa0` (only when `param_5 & 0x400`) | `FUN_0059d9d0`, `FUN_00582890` |
| +7     | 1 | unknown writer                           | `FUN_0059d9d0`, `FUN_0056e430` (×4 = elevation in px) |

## Per-object sprite blit — UNKNOWN

The sprite blit for OBJECTS (not floor tiles) hasn't been located.
Candidates investigated and ruled out:

- `0x0041e4e0` — agent action dispatcher (`agentmagic.cpp`); no rendering.
- `0x004329a0` — NPC script-bytecode interpreter; no rendering.
- `0x00582890`, `0x00572270` — debug overlays / sort-key calc only.

Next places to look:

- `vtbl[2]` of each render-layer object iterated by `FUN_004c72b0`.
  These layer classes own per-object draw and probably call
  `surface->vtbl[7]` with screen X/Y derived from
  `entity.X/Y - camera + something`.
- `FUN_0056e2c0` — called from the drag-and-drop path; might be a
  one-shot sprite blit.
- Function pointers in `DAT_006592dc` (the cube manager — also handles
  sprite display since `WorldManager[5] = DAT_006592dc`).

## Elevation formula — FOUND but draw-time application unclear

The per-position elevation used by the engine is computed by
`FUN_0059fee0` (`div.exe:0x0059fee0`).  It walks an L-shaped search
pattern up-and-left from the target cell (up to 8 cells away), and
for each object whose CUBE covers the search position contributes:

    elevation = collide.<n>[catId].z_height + (layer & 0x3ff)

The MAX (excluding self via the `param_4` exclude key) is returned.

`FUN_0059f8c0` (`div.exe:0x0059f8c0`) is the per-cell helper that
queries one cell's objects.  Key line:

    *param_6 = (int)(short)local_14 + (uVar1 & 0x3ff);
    //         ^ cube[2] = z_height       ^ layer

For Joram-basement reference values (verified):

| Object        | catId | z_height | layer | contribution |
|---------------|------:|---------:|------:|-------------:|
| Wooden table  |   558 |       41 |     0 |           41 |
| Wooden chair  |   131 |       59 |     0 |           59 |
| Wooden wall   |    32 |      148 |     0 |          148 |
| Candle        |  2981 |       26 |    62 |           88 |
| Object 3267   |  3267 |       17 |    42 |           59 |
| Blue flask    |   948 |       15 |    42 |           57 |

For a candle on the table: search up-left from the candle's cell,
finds the table (in cell directly above), max excluding self = 41.
That matches the visible offset between cell-floor Y and the table
top — the candle should render with its base 41 px above world Y.

**However**: `FUN_0059fee0` is only called from the spatial-hash
*removal* path (`FUN_0056d890`, `FUN_0056e0f0`) — recomputing bucket
byte 6 after an object is removed.  It is **not** called per-frame
by any renderer.  The per-frame elevation lookup is much simpler:

## Per-frame elevation lookup — FOUND

`div.exe:0x00427030`:

```c
int FUN_00427030(WorldManager *param_1, int bucketX, int bucketY) {
    uint idx = bucketY * 0x400 + bucketX;
    if (idx < 0x200801)
        return *(byte *)(*(int *)(param_1 + 8) + 7 + idx * 8) * 4;
    return 0;
}
```

So per-frame elevation = `bucket.byte_7 * 4`.

Used in `FUN_00586d60` (item drop / placement render):

```c
iVar13 = param_4 + iVar12;             // worldY
iVar8 = iVar13 + param_2 + param_3;    // worldX + worldY (iso bucket X)
uVar9 = FUN_00427030(iVar8 >> 5, iVar13 >> 5);
iVar12 = iVar12 - (uVar9 & 0xffff);    // worldY -= elevation
```

**This is the formula**: `screen_Y = worldY - bucket.byte_7 * 4`.

## Bucket byte +7 populator — FOUND (terrain only)

`FUN_0056d260` (`div.exe:0x0056d260`) loads `static\height.x<n>` into
the engine's in-memory cell records.  Per-cell layout in
`height.x<n>` is 3 bytes: `{u8 height; u16 flags;}`.  The loader
copies `height` into byte +7 of the corresponding spatial-hash
bucket — see `re_docs/formats/height.md`.

**Crucially**: `height` is the **terrain elevation** only — 0 for
~93% of cells, non-zero only on cliffs/terraces (16, 20, 32, 40, 48,
61, 62 are the only observed non-zero values).  It does NOT carry
table-top / shelf elevation.

For Joram's basement (cell sec=58 cel=123 [table] and cell sec=59
cel=123 [candle/items]) `height.x0` is `0` for both cells.  So the
per-frame lookup `FUN_00427030(bucket).byte_7 * 4 = 0` — and the
engine draws items at `screen_Y = worldY - 0 = worldY`.

**That means top-left at `(worldX, worldY)` IS the engine-correct
draw for items at level-0 elevation cells.**  There is no separate
table-top elevation; items placed on tables in `world.x<n>` are
already given the world-Y the engine wants.  The visual "items on
table top" appearance comes from the sprite art being drawn at the
back-end of the table sprite (which extends far enough down that the
item's small sprite overlaps the visible table-top region).

## Conclusion

Engine-traced render formula now implemented in the viewer:

  tlY = worldY - layer

The 10-bit Layer field in `world.x<n>`'s ord_kind ALREADY
encodes the cumulative pixel elevation of the object.  The
engine (`div.exe:0x005830c0`) iterates SBPutOn entities and
accumulates `max(z_height + runtime_y + 1)` — for each
successive stacked object the runtime_y is set such that the
per-object layer field captures the full accumulated elevation.

**Layer applies to walls too.**  An earlier hack zeroed
elevation for SBLightBlocker objects to "fix" walls that
appeared too high.  That broke door lintels (id=45 layer=112,
SBLightBlocker, no SBNoLookThrough — a thin top-piece sitting
above the doorway).  The lintel got drawn 112 px below its
proper position, appearing at the bottom of the door.  Removed
the hack.

**Terrain elevation NOT applied per-object.**  The per-frame
elevation in `FUN_00427030` resolves at the spatial-hash *bucket*
coordinate `(X+anchorX, Y+anchorY)/32`, not the object's storage
cell.  Approximating with the storage-cell `HeaderByte7 * 4`
shifts indoor items off their tables in cellars where the
basement floor itself carries a non-zero terrain byte (e.g.
Joram's basement cells s=58–59, c=133–135 have b7 = 4–20).
Until the actual draw-time application is traced, the cell's
`HeaderByte7` is captured into `floorCell.TerrainElev` but not
consumed.

**Known gap:** multi-section castle walls that span a terrain
transition (e.g. at world (16695, 7030)) still show a vertical
gap where the wall sprites end and the next section begins
~200 px lower.  The level data has no LightBlocker objects in
that Y range — the engine must extend wall coverage dynamically
based on terrain, but the per-frame world-render call into
FUN_00556a80 is still untraced (FUN_0041e4e0 builds a depth-sort
queue via FUN_004f7b40, but the drainer that produces final
screen X/Y is not yet located).

Empirical verification:
- Wooden table layer=0, z_height=41
- Items on table layer=42 ≈ table.z_height + 1
- Candle on holder on table layer=62 ≈ +holder height
- Bookcase top shelf layer=127 (full stacked column)
- Painting layer=63 ≈ painting cube z_height ≈ 59

Result: candles, bottles, paintings, bookcases, all interactable
items render at engine-correct screen position.

## Still wrong: interactable items, paintings, bookshelf

User confirms after the elevation trace that the *general* scene is
OK but several **specific** objects are still wrong:
- Interactable items on tables (Class > 0 / `SBMakeFloating`).
- Wall paintings.
- Bookshelf rendering (a single object that looks "broken").

User hypothesis (likely correct): interactable items have a separate
render path the engine takes when `SBMakeFloating` (bit 16, 0x10000)
or `Class > 0` is set.

What's been verified about these:

| Field | Where | What we found |
|---|---|---|
| `objects.000 +0x18` `AnimationIndex` | catalogue | Candle (id 2981, layer-0) has 160 (animated via APacked); items class 1..4 are almost always `-1` |
| `objects.000 +0x1c` `SBFlags` bit 16 | catalogue | Set on every interactable item (the `SBMakeFloating` flag) |
| `objects.000 +0x4c` `FloatingImageIndex` + `+0x50` `FloatingListIndex=3` | catalogue | Items reference imagelist 3 (48x48 INVENTORY ICONS — verified by dumping entries 200/180/830/447) |
| `objects.000 +0x70` `FloatingGroup` | catalogue | Mirrors Class (1=armor, 2=weapon, 3=book, 4=potion); 0 for env |

The 48×48 floating images are inventory UI icons, not world sprites,
so the world render of items is still imagelist 0.  But "something"
about the world render is different for `SBMakeFloating` items.

## Next investigation

Find code that branches on `SBFlags & 0x10000` (SBMakeFloating) in a
render context.  Other promising candidates:

- `SBForceBackwall` (bit 5 = 0x20) — likely the wall-painting render
  flag.  Would position painting sprites at the back-wall of a cell
  rather than at the cell's floor Y.
- Bookshelf-as-container: bookshelves likely draw the shelf as a
  layer-0 base sprite plus per-book sprites at higher layers.  If
  one of those sub-sprites is mis-anchored we'd see "broken
  rendering".
- Per-frame animation: candles and similar are animated via
  `AnimationIndex` → APacked.  Static frame 0 is wrong-looking but
  not mis-positioned.

## SBPutOn elevation handler — FOUND

`div.exe:0x005830c0` is the elevation calculator.  Given a target
position and a catalogue ID, returns the max elevation contributed
by any nearby object whose cube overlaps:

```c
short FUN_005830c0(WorldMgr *mgr, int worldX, int worldY,
                   int catId, int *outElev, int selfExclude) {
    psVar9 = cube_for(catId);                       // target's cube
    elev = FUN_00427030(bucketX, bucketY);          // terrain byte_7 * 4
    for each entity in mgr->entities {
        if (entity.catId == selfExclude) continue;
        otherCube = cube_for(entity.catId);
        if (FUN_00471da0(target, other, ...)) {     // cube overlap
            if (objects.000[entity.catId].SBFlags & 0x200000)  // SBPutOn
                elev = max(elev, otherCube.z_height + 1 + otherCube.runtime_y);
            else
                return 0;                            // collision blocks placement
        }
    }
    *outElev = elev;
    return 1;
}
```

Caller `div.exe:0x004a3070` (cursor-with-item-attached render):
```c
elev = 0;
ok = FUN_005830c0(worldX, worldY, catId, &elev, selfId);
if (ok) param_1[9] -= elev;                          // SCREEN_Y -= elevation
FUN_00556a80(..., param_1[8], param_1[9] + scroll_y, ...);  // BLIT
```

Verified for Joram's house:
- Wooden table (id 558) has SBPutOn ✓ — z_height = 41
- Bookcases (976/978/1389) have SBPutOn ✓
- Plates (939), Books (1453), Manuscript (1472) have SBPutOn ✓
- Chair (131), Wall (32) do NOT have SBPutOn

So for a candle/bottle on a table:
- terrain elev = 0 (height.x0)
- table contribution = 41 + 1 = 42 px (assuming runtime_y = 0)
- screen_y = worldY - 42

This is the **engine-traced** elevation formula.  The implementation
needs cube-overlap testing between the item and nearby SBPutOn
objects.  For a simplified static viewer, "overlap" can be approximated
by: same cell or adjacent (the engine's L-shaped 8-cell search in
`FUN_0059fee0`).

## Specific objects in Joram's house and their SB flags

From `world.x0` cells around Joram's spawn, these are the objects
the user reports as visually wrong:

| id | Name             | SBFlags     | Likely category |
|---:|------------------|-------------|-----------------|
|  59| Painting         | 0x00000100  | Wall mount (SBNoLookThrough only) |
| 939| Plate            | 0x00200004  | SBPutOn + Shadow |
|1472| Manuscript       | 0x00200000  | SBPutOn |
|1384| Books            | 0x00000000  | (no SB flags) |
|1385| Books            | 0x00080000  | SBNeedPerfectMatch |
|1386| Scrolls          | 0x00080000  | SBNeedPerfectMatch |
|1388| Books            | 0x00080000  | SBNeedPerfectMatch |
|1389| Bookcase         | 0x00200000  | SBPutOn |
|1453| Book             | 0x00200000  | SBPutOn |
| 976| Bookcase         | 0x00202000  | SBPutOn + SBLightBlocker |
| 978| Bookcase         | 0x00202000  | SBPutOn + SBLightBlocker |
| 964| Mug of water     | 0x00000804  | SBDirectMove + Shadow |
| 237| Dirk             | 0x00010800  | SBMakeFloating + SBDirectMove |
|4180| Minor Health     | 0x00110804  | SBMakeFloating + SBDirectMove + Shadow + SBShowInObjectBox |

The two strongest signals are `SBPutOn` (bit 21 = 0x200000) and
`SBMakeFloating` (bit 16 = 0x10000).  Books, plates, manuscripts,
bookcases all have SBPutOn.  Interactable items have SBMakeFloating.
The painting itself only has SBNoLookThrough — it's likely positioned
via a different mechanism (probably tied to the wall it's attached to).

Bookcases (id 976/978/1389) and book sub-objects (1384/1385/1386/1388/
1453) are placed as separate world objects.  When rendered together
they form a "filled bookcase".  If sub-anchoring is wrong the books
appear floating around the case → "broken bookshelf" visual.

## What this means for the OpenDivine viewer

Until the per-object blit is fully traced:

- Layer-0 (walls / floor / tables / chairs) render correctly with
  top-left at world `(X, Y)`.  Verified against engine screenshots.
- Layer-`>0` (bottles, candles, items) currently render at the same
  top-left, which puts their cell-floor Y at the bottom of the sprite
  → they look ~30 px too low (sit below the table top).
- The fix is **not** an elevation constant; it's whatever the engine
  draw routine does with `entity.Z`, the bucket's `byte_7`, or a
  per-object height field we haven't decoded yet.
