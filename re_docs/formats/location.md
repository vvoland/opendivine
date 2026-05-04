# `location.000` / `location.001` / `location.002` — named coordinate tables

A simple plaintext-headed binary table mapping a string name (a story-script identifier) to three `u32` fields. Three flavours coexist using the same on-disk schema, distinguished by an inline sub-format tag:

| File | Sub-tag | Purpose |
|---|---|---|
| `global\location.000` | `StoryV1.0` | Story locations / teleport targets (e.g. `stps_hero`, `stps_Joram`, `stps_Mardaneus`). |
| `global\location.001` | `TrapsV1.0` | Trap locations. |
| `global\location.002` | `GenerationV1.0` | Generation/spawn locations. |

All integers little-endian.

## Layout

```text
struct LocationFile {
    char magic[23];            // "Divinity LocationsV1.0\0" — 22 chars + NUL, written as a 0x17-byte block
    u32  sub_tag_len;          // length INCLUDING the trailing NUL (10 / 10 / 15)
    char sub_tag[sub_tag_len]; // "StoryV1.0\0" | "TrapsV1.0\0" | "GenerationV1.0\0"
    u32  count;
    Record records[count];
};

struct Record {
    u32  v0;                   // semantic per sub-tag (likely x or id)
    u32  v1;                   // semantic per sub-tag (likely y or aux)
    u32  v3;                   // semantic per sub-tag (likely flags or third coord)
    u32  name_len;             // length INCLUDING the trailing NUL; 0 means "no name"
    char name[name_len];       // ASCII identifier, NUL-terminated; absent when name_len == 0
};
```

The in-memory record stride in the engine is `0x10` bytes (`u32 v0 / u32 v1 / char* name / u32 v3`), but on disk the writer interleaves `v0 / v1 / v3 / name_len / name` — note that `v3` (memory offset `+0xc`) is written *before* the name, even though in memory it follows the name pointer (`+0x8`).

Field semantics (`v0` / `v1` / `v3`) are not yet pinned down; the in-game uses suggest world coordinates plus a flags or scene-id word, but verify against a story script before relying on them.

## Hex dump — `global/location.000` first record

```text
00000000  44 69 76 69 6e 69 74 79 20 4c 6f 63 61 74 69 6f
00000010  6e 73 56 31 2e 30 00                            "Divinity LocationsV1.0\0"  (0x17 bytes)

00000017  0a 00 00 00                                      sub_tag_len = 10
0000001b  53 74 6f 72 79 56 31 2e 30 00                    "StoryV1.0\0"

00000025  42 06 00 00                                      count = 0x642 = 1602

00000029  00 28 00 00                                      record[0].v0  = 0x2800 = 10240
0000002d  30 e4 00 00                                      record[0].v1  = 0xe430 = 58416
00000031  00 00 00 00                                      record[0].v3  = 0
00000035  0a 00 00 00                                      record[0].name_len = 10
00000039  73 74 70 73 5f 68 65 72 6f 00                    "stps_hero\0"

00000043  40 1f 00 00                                      record[1].v0  = 0x1f40 = 8000
00000047  90 0e 00 00                                      record[1].v1  = 0x0e90 = 3728
0000004b  00 00 00 00                                      record[1].v3  = 0
0000004f  0b 00 00 00                                      record[1].name_len = 11
00000053  73 74 70 73 5f 4a 6f 72 61 6d 00                 "stps_Joram\0"
…
```

## Loader citations

```text
div.exe:0x004a0b10   FUN_004a0b10   game init: opens location.000 / .001 / .002 by index, calls FUN_0057c3d0
div.exe:0x0057c3d0   FUN_0057c3d0   constructor: stores path on the location-list object and verifies fopen
div.exe:0x0057bb80   FUN_0057bb80   writer: emits the magic, sub-tag, count, and per-record fields above
```

The writer is the most direct format reference — the reader is split across the
location-list class (loaded lazily in `FUN_0057bfe0` and friends), but produces the same
on-disk layout by construction.
