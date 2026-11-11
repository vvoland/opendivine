# `sound\music.dat` — music tracks & zones

The music database: the named tracks the engine can stream, and the
named **music zones** that select which track plays where. The
streaming runtime that consumes this (the `SMMusicManager` fade/duck
state machine) is in [`../sound-runtime.md`](../sound-runtime.md); this
doc is the data file behind it.

All integers little-endian.

## Section 1 — track table ✅

```text
u32  count                    (= 43)
Track[count]:
    u32   name_len
    char  name[name_len]      // short id name: "1", "12", "17cAmMix2", "AmbCreepP"
    u32   file_len
    char  file[file_len]      // the .ogg, e.g. "1.ogg", "amb creepP.ogg"
    u32   field               // 0 in every shipped record
```

43 tracks. The `name` is a short handle; `file` is the streamed clip
(under `wav\music\` per [`../sound-runtime.md`](../sound-runtime.md),
played via FMOD `FSOUND_Stream_*`). `name` and `file` can differ
(`AmbCreepP` → `amb creepP.ogg`). Parsing the 43 records consumes
1417 bytes, then **section 2 begins**.

## Section 2 — location tracks 🟡

```text
u32  count                    (= 22)
Track[count]:
    u32   name_len
    char  name[name_len]      // location name, e.g. "BehindDaBridge", "Cellar", "Church"
    u32   file_len
    char  file[file_len]      // "<name>.ogg"   (no trailing field, unlike section 1)
```

A second list of **22 location-named tracks** (`BehindDaBridge →
BehindDaBridge.ogg`, `Cellar → Cellar.ogg`, `Church`, `Dungeon`, …) —
the place-specific music, distinct from section 1's numbered/ambient
tracks. (Correction: an earlier pass mislabelled these as "zones"; they
are tracks. The zone/region → track selection lives in the `MusicV1.0`
region table, `region.001`, [`region.md`](region.md), which references
these track names.) Note the record has **no trailing `u32`** that
section 1 carries.

Parsing section 2 reaches offset 2063; the **third section** is the
zone map below.

## Section 3 — zone → music and ambient bindings ✅

```text
u32  count                    (= 153 zones)
Zone[count]:
    u32   name_len
    char  name[name_len]      // "Aleroth", "Aleroth_Cellar", …; no NUL
    List  lists[5]
    u32   flag               // observed 0/1; meaning not established
List:
    u32   count
    Assignment[count]:
        u32   label_len
        char  label[label_len]
        f32   volume         // playback gain, NOT a selection weight
```

Zone names match the **`MusicV1.0` region table** (`region.001`, [`region.md`](region.md)).
L0 holds primary music; L1 is reserved and unused; L2 holds timed overrides, untriggered in the shipped engine.
L3 and L4 hold day and night ambients respectively, not alternate music playlists.
Music lists resolve against section 1; ambient lists resolve against section 2.
An empty night ambient list falls back to the day list.
See [sound.md](sound.md) for the reader functions and runtime evidence.

For `Aleroth`, L0 is `[("4", 0.2), ("forest", 0.2)]`, L1/L2 are empty, L3 is `[("GenDay01", 1.0)]`, and L4 is `[("GenNight", 1.0)]`.
The earlier two-list, weighted day/night-music interpretation was incorrect.

## Status

- Section 1 (tracks) ✅ — `u32 count(43)` + `{name, .ogg file, u32
  field}` records; verified.
- Section 2 (location tracks) ✅ — `u32 count(22)` + `{name, .ogg}`
  records (no trailing field); 22 place-named tracks. *Corrects an
  earlier "zones" mislabel.*
- Section 3 (zone map) ✅ — `u32 count(153)` zones with five assignment lists and a trailing flag; each assignment stores a label and volume.
- Region → track link ✅ (by name) — the `MusicV1.0` regions (`region.001`) match zone names; L0 supplies music and L3/L4 supply day/night ambients.
- The `field` (`+after file`, always 0) ❓ — a track flag/type, unused
  in the shipped data.
