# `static\osinames.000` & `static\osiobjects.000` — Osiris static snapshots

Two small fixed-record tables under `main\startup\static\` that pair the compiled Osiris story ([`../osiris.md`](../osiris.md)) with the live world's named objects.
**Both parse byte-exact** (no baked-pointer heap-dump caveat, unlike `books.000`).

## `osiobjects.000` — object handle → index (byte-exact)

```text
u32   count = 1808
u32   0                      (reserved / header pad)
Record[count]:               (8 bytes)
    u32  objectHandle        e.g. 0xda5d, 0x697, 0x620 …
    u32  index               sequential 1 … count
```

`1808 × 8 + 8 = 14472` = exact file size. The **1808 count matches the
`OBJECT`-type (type 5) DIVObject count** in `story.000`
([`../osiris.md`](../osiris.md) DIVObject table), so this maps each
Osiris object's runtime handle to its dense index.

## `osinames.000` — id → name (byte-exact; append log)

```text
u32   count = 2060
u32   0                      (reserved)
Record[count]:               (36 bytes — u32 + char[32])
    u32   id                 pairing key (NOT sequential); 0xFFFFFFFF = tombstone
    char  buf[32]            "<name>\0" then stale buffer tail + 0xCD fill
```

`2060 × 36 + 4 = 74164` = exact file size. **Corrections to the first
decode:** the file is an **append log**, not a dense table — 2060
records carry **1807 distinct ids** plus **253 `0xFFFFFFFF`
tombstones**, and records pair by their `id` field, not by position.
And the "second template/class string" **does not exist** — what looked
like `ct`/`d_grave2` codes are **stale buffer tails** from previous
(longer) names, left behind by the 32-byte fixed write before the
`0xCD` fill. Only the first NUL-terminated string (the object's
instance name) is real. Together with `osiobjects.000` this is the
Osiris ↔ world-object binding the engine restores at load.

**Handle space ✅ (PROVEN).** `osiobjects.000` handles are
**`objects.x<n>` 28-byte record indices** — the same space as the
dynamic cell-entry bits 12..31 (below). Proof by byte-exact round trip:
Osiris-owned instance records carry `s_flags` bit 8 (`sb_osiris`) with
the **Osiris key = osiobjects index − 1** in their value pool; scanning
all five maps resolves **1750/1808 entries with zero cross-map
ambiguity** (x0 1046, x1 299, x2 320, x3 78, x4 7), and the 58
leftovers decompose exactly into 27 stale duplicate rows + 31 removed
instances. Null controls (±1 shift, randomized handles) collapse to
chance.

## Runtime: the WORLD data-file registry (global `0x750d38`)

The per-map dynamic files are owned by a single **`.\WORLD\` data-file
registry** — a global object at **`div.exe:0x750d38`**, constructed in
`main` (`0x40db90`) by **`fcn.0057c650`**. The ctor `strdup`s the canonical
relative path of every runtime data file into a **fixed field** of the
registry object (`esi`), so each file has a stable slot the rest of the
engine resolves through; the `.x<n>` per-map variants are produced from
these defaults by `sprintf("…%d", n)` in the transfer helpers below.
Verified field layout (from the ctor's `strdup` stores):

```text
reg+0x00  dynamic\world.x0      per-map cell/object grid    ([world.md](world.md))
reg+0x04  dynamic\objects.x0    per-map object INSTANCES    (28-byte records — decoded below ✅)
reg+0x08  dynamic\extfree.x0    per-map object free-list    (decoded below ✅)
reg+0x0c  dynamic\items.000     item instances              ([items.md](../items.md) — byte-exact ✅)
reg+0x14  dynamic\minventi.000  merchant? inventory index   (TRamFile pair,
reg+0x18  dynamic\minventb.000    [inventory.md](../inventory.md))
reg+0x1c  dynamic\oinventi.000  object? inventory index
reg+0x20  dynamic\oinventb.000
reg+0x24  dynamic\inventi.000   agent inventory index
reg+0x28  dynamic\inventb.000
reg+0x40  dynamic\minvent       prefix slots (mgr ctor sprintf "%si.000"/"%sb.000")
reg+0x44  dynamic\oinvent
reg+0x48  dynamic\invent
reg+0x10  dynamic\books.000     book texts                  ([books.md](books.md) — write-only slot)
reg+0x2c  dynamic\height.x0     per-cell height/flags       ([world.md](world.md))
reg+0x30  dynamic\shroud.x0     per-map fog-of-war          ([shroud.md](shroud.md))
reg+0x34  static\osiobjects.000 (this doc)
reg+0x38  static\osinames.000   (this doc)
reg+0x50  global\               cross-map globals dir
reg+0x54  global\               (second global slot)
reg+0x58  dynamic\              dynamic dir prefix
```

(Earlier notes here put a "file-manifest builder at `~0x57c500`" interning
paths to handles via `fcn.005e5eec`; **`fcn.005e5eec` is just the
`MSVCR90._strdup` import thunk** — there is no handle interner and no
generic virtual-stream deserializer. The owner is this `0x750d38`
registry of strdup'd path strings — `~50` loaders take it as `this`
(`mov ecx, 0x750d38`) to resolve a file by slot. `osiobjects.000` /
`osinames.000` are two entries in the same set.)

### The `.x<n>` files are managed as opaque blobs (`.\WORLD\Compress.cpp`)

Tracing the per-map files end-to-end, the whole `0x57c…0x57e` /
`0x573…` cluster moves `world.x<n>` / `objects.x<n>` / `extfree.x<n>` /
`height.x<n>` / `shroud.x<n>` / `mapv.<n>` **as whole files**, never
parsing their records:

```text
fcn.0057c650   registry ctor — strdup canonical paths into the fields above.
fcn.0057cb00   Merged-Map IMPORT — open local\%s\%s, check "Divinity Merged
   /fcn.0057dc10   Map V1.0" header, rebuild the registry, unpack group.0 /
                group.c0 / group.%d / group.c%d packed archives into dynamic\.
fcn.005732a0   .\WORLD\Compress.cpp — save-state bundler: slurp main\game.sav /
                local\%s\game.%s, then copy/replace dynamic\world.%s /
                objects.%s / extfree.%s … for the active save suffix.
fcn.0057d580   copy the whole .x<n> set ("Can't copy %s to %s (Reason %s)").
fcn.0057d800   "Transferring maps from" — transfer/copy the set across dirs.
fcn.0057eb10   scratch cleanup — unlink scratch\{world,objects,extfree,height,
                shroud}.x<n>.
```

The `group.c0` / `group.c%d` ("**c**" = compressed) archives are
Huffman-packed: `Compress.cpp`'s pack (`fcn.005738d0`) / unpack
(`fcn.00572d70`) run each file through the `.\MISC\Hufmann.cpp` codec
(encode `fcn.004f9ea0` / decode `fcn.004fa1e0`) — see
[`hufmann.md`](hufmann.md).

So **entering / leaving a map and saving** is file plumbing here: the
`.x<n>` blobs are merged, copied between `dynamic\` ⇄ `scratch\` ⇄
`local\<save>\`, and bundled into the merged-map / `game.sav` archives.

## `objects.x<n>` — per-map object instances ✅ (RESOLVED, byte-exact)

*(This resolves the long-standing "heap-dump, replay-the-deserialize"
🟡 — which turned out to be a misreading: there are **no pointers** in
this file, and the `0xffffffff` runs are just free slots.)*

The file is a header-less **random-access record heap**: a flat array
of **28-byte (0x1c) object-instance records**, addressed by
`handle = record index`. It is opened `"rb+"` and patched in place, one
record per fseek/fread/fwrite, through an 8 KiB page cache — it is
never "deserialized" as a stream at all.

```text
Record (28 bytes); a free record is 28 × 0xFF:
    +0x00  u32     s_flags      same bit space as the objects.000 catalogue
                                flags_a (s_key, s_door, s_chest, s_value…);
                                read with the same unpacker fcn.005918b0,
                                setters 0x591920/0x591940/0x591b60
    +0x04  u8[16]  value pool   packed per-set-bit values (key id, lock,
                                function params…). Slot widths: bits
                                0/3/4/5/6/8 → 2 bytes, bits 1/2 → 1 byte;
                                values are MSB-first.
    +0x14  u16     x            world coord; cell = x>>6, sub = x & 0x3f
    +0x16  u16     y            (matches world.x sub_x/sub_y granularity)
    +0x18  i16     elevation/anchor offset (mirrors live CObject+0x18)
    +0x1a  u16     kind         catalogue index into static\objects.000
                                (validated < 7208 everywhere)
```

Validated against **all five shipped files** (sizes ≡ 0 mod 28):
`objects.x0` 73,734 records (510 free), `.x1` 60,966 (124), `.x2`
85,908 (2,758), `.x3` 20,775 (14), `.x4` 55,782 (1,031).

**`extfree.x<n>` ✅** — `{u32 count; u32 slots[count]}`, each slot a
free handle or `0xffffffff` (an already-reallocated slot of the
in-memory free array). Cross-validated: the non-(−1) slots equal
**exactly** the all-FF record set of the paired `objects.x<n>`, for
all 5 partitions.

Key functions (`.\WORLD\objects.cpp`, manager singleton `[0x658bdc]`):
ctor `fcn.0058a250` (boot `0x499c5f` / reload `0x4a0c4d`; loads the
catalogue `fcn.00586550`, allocs the 0xd8000 page cache, opens
`[0x750d3c]` = `reg+4` `"rb+"`, loads extfree via `fcn.00581cc0`);
`fcn.005863c0` GetRecordCount = `ftell(END)/28`; `fcn.005863f0`
ReadRecords(h,n) = `fseek(h·28)+fread`; `fcn.005820c0`/`fcn.00582170`
write-record; `fcn.00581d60` record→live-object; `fcn.00586c60`
FreeRecord (wipe to FF); `fcn.0058a3e0` load-time repair scan (frees
records with kind ≥ catalogue count).

**Grid link (refines [world.md](world.md)):** in the dynamic state, the
cell's 8-byte object entry's first u32 = bits 0..5 `sub_x`, 6..11
`sub_y`, and **bits 12..31 = the `objects.x` handle** (`shr 0xc` at
`0x585e2d`, `0x580600`) — not a 4-bit flags index.

## Status

- `osiobjects.000`/`osinames.000` ✅ — fully decoded, byte-exact to EOF;
  simple count + fixed-size records (8 B handles / 36 B name records),
  directly reimplementable.
- Field semantics 🟡 — `osinames` second string confirmed a
  template/class code by sample; the exact `osiobjects` handle space
  (vs the `story.000` symbol ids) is the only nuance left.
- WORLD data-file registry ✅ — global `0x750d38` (ctor `fcn.0057c650`,
  `.\WORLD\`), verified path-field layout (table above); no interner,
  no generic deserializer (`fcn.005e5eec` = `_strdup`). The `.x<n>`
  lifecycle (merge / transfer / compress / scratch-clean) is fully
  traced as **opaque-blob** file plumbing (`Compress.cpp`).
- `objects.x<n>` + `extfree.x<n>` ✅ — 28-byte record heap + free-list,
  decoded and validated byte-exact against all five shipped pairs
  (table above). The former "virtual-stream heap-dump" classification
  is retracted.
