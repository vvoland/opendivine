# Hero (player + NPC) sprite composition — engine trace

What the engine does to render a "person" character (player and most
human NPCs share the same code path). Traced from `div.exe`'s
`.\NPC\Combine.cpp` and `.\AGENTS\agents.cpp`.

## Files

```
static\heroes\<class>.key                         text directory
static\heroes\<class>{A,B,C,D,E}.{idc,bic}        5 layered variant banks
```

Six classes ship: `surm` / `surf` (survivor m/f), `warm` / `warf`
(warrior), `wizm` / `wizf` (wizard). Each character class is
**always rendered as a stack of up to 5 variant layers**, never as a
single sprite — `surmA.bic` alone is just the legs+cloak piece.

| Variant | Role             | 4th-char (equipment slot)                  |
|---------|------------------|--------------------------------------------|
| A       | legs + cloak     | `local_1c[0]` — legs equipment letter or `0` |
| B       | torso + arms     | `local_14[0]` — torso equipment letter or `0` |
| C       | helmet           | `local_2c[2]` — only added if helmet equipped |
| D       | head / face / hair | `local_c[0]` — face letter or `0`        |
| E       | weapon           | `local_24[0]` — only added if weapon equipped |

The `.idc` hotspot of each variant lands on the character's world
position; the engine's combiner re-anchors them so the layers stack
correctly (see `MakeAnimationDirectionsFromKeys`).

## `.key` group naming

A group name is `M{action_letter}{variant_letter}{equipment_letter}`:

| Variant | Group prefix examples | Notes                                    |
|---------|-----------------------|------------------------------------------|
| A       | `MAA0`, `MBA0`, `MQA0`, `MUA2` | 17 actions × 3 directions = 51 groups |
| B       | `M1B0`, `M1B3`, `M2B0` | 2nd char is a NUMBER                     |
| C       | `MACA`, `MACB`, `MACE` | 4th char varies A..E                     |
| D       | `M1D0`, `M2D0`         | 2nd char is a number                     |
| E       | `MAE1`, `MAE2`         | 4th char is a number                     |

So **the 2nd-char "action" letter is variant-specific** — `MAA0`
exists, `MAB0` does NOT. The engine's `FUN_00439b70` knows the
mapping and substitutes the right letter per variant per action.

## Per-frame layout inside a `.key` group

Each group contains all engine directions concatenated.  The
direction count is **per anim slot**, set per character class
(see "Direction count source" below).  For the player class
("Hero") it is 20 for most slots, 5 for slot 4 (E weapon-only),
0 for disabled slots (7/10/14/15).

```text
group MBA0 (320 frames)            ← idle ('B'), 320 / 20 dirs = 16 fr/dir
├── engine dir  0 (N) :  frames   0..15
├── engine dir  1     :  frames  16..31      (~18° each, CCW from N)
├── engine dir  5 (W) :  frames  80..95
├── engine dir 10 (S) :  frames 160..175
├── engine dir 15 (E) :  frames 240..255
└── engine dir 19     :  frames 304..319

group MAA0 (480 frames)            ← walk  ('A'), 480 / 20 dirs = 24 fr/dir
```

The 4th-char of the group name is the **equipment letter** (e.g.
`MAA0` = legs equipment 0 = none, `MAA3` = legs equipment 3).  The
composer (`FUN_00439b70` / `character_compose.go`) stamps it in
based on what the character is wearing.

## Engine call chain

```text
agents.cpp:
  FUN_0042c8e0(agent, "static\\heroes\\<class>")    // load triplet on agent spawn
    → FUN_0050bb10()                                  // load the 5 .bic+.idc pairs
        → for v in A..E:
             FUN_004e9fe0(<class><v>.bic, <class><v>.idc, stride=0x28, ...)
        → allocate 19 animation slots × 5 frame entries each
    → FUN_0050a840(<class>.key)                       // parse .key text directory

per-tick render:
  FUN_0043afc0(agent, anim_index)                    // pick + composite layers
    ├── allocate local_64[5] of 5-byte name buffers
    ├── FUN_00439b70(agent, anim_index, local_64, &count)  // build per-layer names
    │     based on action (anim_index → cVar4) + helmet + face +
    │     legs equipment + torso equipment + weapon
    └── FUN_0050ac30(model, local_64, count, output_frames, ...)
                                                     // MakeAnimationDirectionsFromKeys
          per direction:
            per layer:
              decompress+lookup the layer's frame
              widen the per-direction bounding box
              shift the layer's spans into the shared bbox
            emit one COMBINED sprite per direction (final blit src)
```

## Animation index → action letter (`FUN_00439b70`)

| `anim_index` | `cVar4` (action) | Notes                              |
|-------------:|------------------|------------------------------------|
|  0           | `B`              |                                    |
|  1           | `A`              |                                    |
|  2           | `Q` if no helmet, else helmet's `[0]` byte |  default idle uses `Q` |
|  3           | `D`              |                                    |
|  4           | `E`              | `local_44 = 1` (weapon shown only) |
|  5           | `F`              |                                    |
|  6           | `H` (variants)   |                                    |
|  7           | `P` (only if `local_40 == 2`)             |              |
|  11          | `G`              |                                    |
|  12          | `C`              |                                    |
|  13          | `Z`              |                                    |
|  16          | `J`              |                                    |
|  17          | `M` or `K`       |                                    |
|  18          | `U`              |                                    |

For variants B and D, the 2nd char is then mapped through a complex
table that depends on the helmet letter — e.g. `cVar4='G'` with
helmet letter `'E'` becomes `'1'`, with `'N'` becomes `'2'`, etc.
See the switch ladder around `LAB_0043a31b`.

## Status for OpenDivine

All five-layer composition is **working** for unequipped warrior /
survivor / wizard (m and f).  The composer (FUN_00439b70 port) is
in `cmd/divine/character_compose.go`; it produces .key group names
per anim slot + equipment.  Layer placement uses the corrected IDC
field layout (see below).  Walk animation cycles through engine
direction blocks correctly (engine_dir = (our_Dir + 2) % 8).

Open items:
- Equipped paths (layer C helmet, layer E weapon) — `AttachPairs`
  semantics still TBD; need to confirm which slot anchors what.
- NPC spawning — the pipeline is the same, the placeholder
  `g.npcs` slice in main.go just isn't filled yet.
- Walk-cycle pacing (`AnimTick >= 6` in Step is arbitrary; the
  engine likely ties advancement to movement distance).

## Corrected .idc field layout

Earlier iterations of this doc and the loader interpreted the .idc
record as `(Width, Height, HotspotX, HotspotY)` at offsets +8..+15.
That was wrong.  The engine (`FUN_0050ac30` lines 224-238) treats:

| Offset | Field   | Type         | Meaning                                |
|-------:|---------|--------------|----------------------------------------|
| +0     | Offset  | uint32       | byte position in group decomp buffer   |
| +4     | Size    | uint32       | byte size of frame                     |
| +8     | XMin    | int16        | bbox left edge in **composite** coords |
| +10    | YMin    | int16        | bbox top edge in composite coords      |
| +12    | Width   | uint16       | bbox width                             |
| +14    | Height  | uint16       | bbox height (also = line-record count) |
| +16    | AttachPairs[12]int16 | (see below)                  |

There is **no separate hotspot field**.  The agent's world position
`(X, Y)` corresponds to the per-class anchor `(CX, CY)` in
composite coords (set by `FUN_0050bb10`), and a frame's world
top-left is therefore `(X + XMin − CX, Y + YMin − CY)` with the
sprite occupying `Width × Height` pixels.  All layers (A, B, C, D,
E) use the same formula — the natural overlap between layers comes
from the artists drawing each layer with overlapping XMin/YMin
ranges.  No seam-snap is needed.

This single fix eliminated the gap-between-kilt-and-legs that all
prior iterations had been trying to compensate for empirically.

## .idc trailing 12 int16s — auxiliary attach points

Each .idc record's last 24 bytes are 6 (int16, int16) pairs in
composite coords (-1 = unused).  Variant A leaves them all -1;
B owns pairs 0/1/2, C owns 4/5, D owns 3, E owns none.  These
pairs are NOT used for the basic per-layer placement — that's
handled by the IDC's XMin/YMin fields.  They appear to be
auxiliary attach points (likely for cross-layer effects like a
weapon held in the torso's hand).  Concrete role TBD.

### Direction enum mismatch (engine vs ours)

The engine stores `N` directions concatenated within each `.key`
group, **counter-clockwise from north**.  `N` is per anim slot
(see "Direction count source" below); for the player class
`N = 20` for most slots → cardinals at indices 0/5/10/15
(N/W/S/E) and 18° per direction.

Our internal `Character.Dir` runs **clockwise from east** with
8 steps: `0=E, 1=SE, 2=S, 3=SW, 4=W, 5=NW, 6=N, 7=NE`.

Translation (works for any `N` divisible by 8 at the cardinals;
diagonals round to the nearest engine index):

```go
engine_dir = (3*N/4 − our_Dir * N/8 + N) mod N
```

Verified by forced-`Dir` screenshots of the warrior + wizf female
+ survivor with the corrected IDC layout.

### Direction count source

The per-anim-slot direction count is **not constant**.  It comes
from the `AgentClasses` block of `main\startup\data.000`,
specifically from byte `+0x4c+animSlot` of class 0 ("Hero" — the
player) — see `re_docs/formats/savegame.md` and `pkg/assets/agentclass`.

For the player the array is:

```text
slot:  0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15 16 17 18
       B  A  Q  D  E  F  H  P              G  C  Z         J  M  U
count: 20 20 20 20  5 20 20  0 20 20  0 20 20 20  0  0 20 20 20
```

Slot 4 (`E`, weapon-only display) uses 5 dirs.  Slots 7/10/14/15
are 0 = disabled.  Engine upper bound is `0x14 = 20`
(`FUN_0050ac30:0x69` panics with "Too many directions" otherwise).

The editor source files `dat\animidx.dat` and `dat\npclist.sng`
that originally produced these values are NOT distributed with
Steam — only the compiled binary `data.000` ships.

### Idle anim plays ping-pong, walk loops

Empirically the per-direction frame block of an idle slot reads
as "cut off" if hard-reset to frame 0 at the loop boundary; it
ping-pongs (`0..N-1`, then `N-2..1`, then `0..`) for a smooth
breath cycle.  Walk and other continuous slots loop normally.
`pingPongOrLoop()` in `cmd/divine/character.go` selects per
`AnimSlot`.

### Slot 0 = unarmed idle, slot 6 = armed idle

`Character.Step()` chooses `AnimSlot = 0` (`B`, unarmed) when
`Equip.Weapon == 0`, else `AnimSlot = 6` (`H`, armed) when
standing still; `AnimSlot = 1` (`A`, walk) when moving.  Slot 11
('G') and slot 2 ('Q') are NOT idle — slot 11 is a move-style
anim, slot 2 is the unarmed punch attack.

## Per-class hardcoded attach defaults

`FUN_0050bb10` (the 5-bank loader) has a switch on the class path
that writes 6 ushorts into a stack array, then passes that array to
`FUN_00471bb0` for storage on the model.  Three (x,y) pairs:

| Class | pair[0]    | pair[1]   | pair[2]   |
|-------|------------|-----------|-----------|
| surm  | (90, 158)  | (0, 32)   | (88, 32)  |
| surf  | (88, 146)  | (0, 32)   | (88, 32)  |
| warm  | (90, 154)  | (0, 32)   | (88, 32)  |
| warf  | (84, 150)  | (0, 32)   | (88, 32)  |
| wizm  | (94, 192)  | (0, 32)   | (88, 32)  |
| wizf  | (82, 184)  | (0, 32)   | (88, 32)  |

pair[0] is class-specific (tracks character standing height — wizard
hat ≫ warrior helm ≫ survivor); pair[1] and pair[2] are
class-invariant.  These are likely **default attach offsets** used
when a per-frame .idc attach slot is unset (-1), so the combiner
always has a fallback anchor for the head-shape (pair[0]) and
upper/lower hand grip (pair[1]/[2]).

Engine refs:
```text
div.exe:0x0050bcde   per-class switch end (LAB_0050bcdf in decomp)
div.exe:0x00471bb0   FUN_00471bb0 — stores the 6-ushort array on model
```

## Loader citations

```text
div.exe:0x0042c8e0   FUN_0042c8e0   agent character setup — calls Combine + parses .key
div.exe:0x0050bb10   FUN_0050bb10   load 5 .bic+.idc per class, allocate 19 anim slots
div.exe:0x0050ab20   FUN_0050ab20   per-anim-slot init (5 frame entries each)
div.exe:0x0050a840   FUN_0050a840   .key text parser ("Max size", "Center", "$,#,#")
div.exe:0x0050aa50   FUN_0050aa50   per-block LZO decompress (heroes.md describes this)
div.exe:0x0050ac30   FUN_0050ac30   MakeAnimationDirectionsFromKeys — the combiner
div.exe:0x0043afc0   FUN_0043afc0   per-tick: build layer names + run combiner
div.exe:0x00439b70   FUN_00439b70   anim_index + equipment → 5 layer .key names
div.exe:0x004e81a0   FUN_004e81a0   shared loader for .bic+.idc pairs (also APacked)
```
