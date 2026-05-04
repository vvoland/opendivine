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
    i16   x_extent;   // [3] right-edge offset from anchor used in AI centre-X:
                      //     FUN_00448020 computes centre_x = avg(anchor_x + width/2, anchor_x + x_extent)
    i16   z_height;   // [4] vertical Z extent of the 3D cube above ground (tiles/units);
                      //     range 1–175 in shipped data; not yet seen in a confirmed consumer
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
```

## Status

- File-level layout ✅ — fixed 16-byte stride; counts validated against shipped CPacked imagelists.
- i16[0] = anchor_x 🟡 — initial sprite-relative offset in file; overwritten with world X at runtime.
- i16[1] = anchor_y 🟡 — same; overwritten with world Y at runtime.
- i16[2] = rt_timer ✅ — RUNTIME only; 0 in file; confirmed from `FUN_004eca40`.
- i16[3] = x_extent 🟡 — right-edge bound used for AI centre-X; confirmed read in `FUN_00448020`;
  represents the cube's rightward reach from anchor. Exact geometric meaning vs i16[5] TBD.
- i16[4] = z_height 🟡 — vertical Z extent; not yet seen in a confirmed consumer function.
- i16[5] = width ✅ — collision cube width; halved for screen centre calc (`FUN_004eca40`).
- i16[6] = type ✅ — 0/1/2 in file; set to 2 at runtime by `FUN_0043b2b0`.
- i16[7] = flags ✅ — 0 in file; bit0 set by `FUN_00471c00`; |= 0x10 by `FUN_004eca40`.
