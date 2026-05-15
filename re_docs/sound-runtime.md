# `div.exe` sound runtime

How `div.exe` plays audio through FMOD. Audio config files (the
`sound\*.dat` triplet) are documented in
[`formats/sound.md`](formats/sound.md); this doc covers the runtime
**code** that consumes those tables.

`fmod.dll` is UPX-packed at rest, but the engine resolves FMOD via
the **static import table**, not `GetProcAddress`. UPX unpacks the
DLL in-place at load time, after which the IAT thunks point at the
real FMOD entry points. So the FMOD surface area `div.exe` actually
uses can be enumerated directly from `div.exe`'s imports.

## 1. FMOD import surface (statically imported)

Thunk table starts at `div.exe:0x005a4830` (each entry is
`JMP dword ptr [IAT slot]`). All 33 FMOD symbols `div.exe` uses:

| Function | Thunk | IAT slot | Notes |
|---|---|---|---|
| `FSOUND_GetCPUUsage` | `0x005a4832` | `[0x006064d4]` | Debug overlay |
| `FSOUND_3D_Listener_SetRolloffFactor` | `0x005a4838` | `[0x006064d0]` | Init from `sound.cfg` |
| `FSOUND_3D_Listener_SetDistanceFactor` | `0x005a483e` | `[0x006064d8]` | Init from `sound.cfg` |
| `FSOUND_IsPlaying` | `0x005a4844` | `[0x0060654c]` | |
| `FSOUND_SetVolume` | `0x005a484a` | | |
| `FSOUND_StopSound` | `0x005a4850` | | Channel stop |
| `FSOUND_3D_Update` | `0x005a4856` | | Per-tick at end of `FUN_00547ca0` |
| `FSOUND_3D_Listener_SetAttributes` | `0x005a485c` | | Per-tick listener pose update |
| `FSOUND_Stream_Play` | `0x005a4862` | | Music streams |
| `FSOUND_Close` | `0x005a4868` | | Shutdown |
| `FSOUND_SetReserved` | `0x005a486e` | | Init: reserves 3 channels |
| `FSOUND_GetError` | `0x005a4874` | | |
| `FSOUND_Init` | `0x005a487a` | | Mix rate fixed at `0x5622` (= 22050 Hz) |
| `FSOUND_SetMaxHardwareChannels` | `0x005a4880` | | From `sound.cfg+0xc` |
| `FSOUND_SetMinHardwareChannels` | `0x005a4886` | | Hard-coded `8` |
| `FSOUND_SetDriver` | `0x005a488c` | | From `sound.cfg+0x4` |
| `FSOUND_SetOutput` | `0x005a4892` | | Hard-coded `2` (DSOUND) |
| `FSOUND_File_SetCallbacks` | `0x005a4898` | | Custom file I/O hook |
| `FSOUND_Stream_Close` | `0x005a489e` | | |
| `FSOUND_Stream_Stop` | `0x005a48a4` | | |
| `FSOUND_Stream_SetEndCallback` | `0x005a48aa` | | |
| `FSOUND_Stream_OpenFile` | `0x005a48b0` | | Music load (mode `0x2130`) |
| `FSOUND_Reverb_SetProperties` | `0x005a48b6` | | From `reverbs.dat` preset |
| `FSOUND_Sample_Free` | `0x005a48bc` | | |
| `FSOUND_Sample_GetLength` | `0x005a48c2` | | |
| `FSOUND_Sample_SetMinMaxDistance` | `0x005a48c8` | | 3D attenuation per sample |
| `FSOUND_Sample_Load` | `0x005a48ce` | | SFX bank load |
| `FSOUND_SetPaused` | `0x005a48d4` | | |
| `FSOUND_PlaySoundEx` | `0x005a48da` | | The main SFX trigger |
| `FSOUND_3D_SetAttributes` | `0x005a48e0` | | Per-channel 3D position |
| `FSOUND_Sample_GetMode` | `0x005a48e6` | | |
| `FSOUND_SetPriority` | `0x005a48ec` | | |

`FSOUND_PlaySoundEx` is wrapped by three play helpers
(`0x0054f3c0`, `0x0054f400`, `0x0054fa60`) that all set volume +
unpause; `FSOUND_Stream_*` is used only for the music subsystem.

The engine never calls `LoadLibraryA("fmod.dll")`. There is one
`GetProcAddress` site (the only one in the `.text`) at
`0x005a8b15`, which resolves *Windows*-API symbols at runtime, not
FMOD. The string `"fmod.dll"` at `0x00647428` is the import-table
descriptor — referenced only via the PE loader, with no code xref.

## 2. Sound bank load

Top-level sound init is `CDivSoundManager::Init` at
`div.exe:0x0054a380` (`.\SOUND\DivSoundManager.cpp`). Sequence:

1. `FSOUND_SetOutput(2)` → DSOUND backend.
2. `FSOUND_SetDriver(cfg.driver)`.
3. `FSOUND_SetMinHardwareChannels(8)` /
   `FSOUND_SetMaxHardwareChannels(cfg.max)`.
4. `FSOUND_Init(0x5622, cfg.channels, 0)` — mix rate 22050 Hz.
5. `FSOUND_3D_Listener_Set{Distance,Rolloff}Factor(cfg.*)`.
6. `FSOUND_SetReserved(3)` — reserves channel 3 for music streams.
7. `FUN_00549ac0()` — `nsound.dat` parser
   (`.\SOUND\DivSoundManager.cpp` line `0x29f`). Schema in
   [`formats/sound.md`](formats/sound.md) §`nsound.dat`.
8. `FUN_00553180()` then `FUN_00552900()` — `SMMusicManager`
   constructor + `sound\music.dat` loader
   (`.\SOUND\SMMusicManager.cpp`).
9. Reverb load happens via separate path (see §3 below).

`nsound.dat` records reference WAV paths like
`\\WAV\\Impact & Swoosh\\Bodyfall01Slide.wav`; each path is
resolved to a file handle by `FUN_0054eaa0` (`SMSoundCache::Lookup`)
which reads the WAV bytes from `dat\sound.cmp` (the obfuscated
asset archive — see `cmp.md`).

`SMMusicManager::Load` (`FUN_00552900`) opens `sound\music.dat`
sequentially: 43 tracks → 22 ambients → 153 region-bindings, then
synthesises a "Default" region from `sound\reverbregions.dat`'s
`REGION_*` enumeration.

The reverb subsystem is loaded by `FUN_0054a3xx` calls (siblings of
`SMMusicManager::Load` in the init chain) reading `reverbs.dat` and
`reverbregions.dat`; each preset is pushed to FMOD via
`FSOUND_Reverb_SetProperties`.

## 3. Per-cell ambient sound (`SBAmbientSound`)

`sb_ambient_sound` is the 13th of the `sb_*` flag bits enumerated in
the editor column header at `0x0061fa58` — bit index 12, mask
**`0x1000`**, on the `objects.000 +0x1c` SBFlags word
(`re_docs/formats/objects.md`).

The per-frame ambient walker runs alongside the music tick. Inside
the per-tick `FUN_00547ca0` (see §5 below) the engine drains three
parallel "live source" lists out of `SMMusicManager`:

```text
FUN_005519f0(mgr + 0x88)   // music slot
FUN_005519f0(mgr + 0xa4)   // ambient slot
FUN_005519f0(mgr + 0xc0)   // interface slot
```

Each slot holds zero or more `SMSoundNode` records (RTTI
`.?AVSMSoundNode@@` at `0x006540a0`, parent class `SMAmbientEntry`
at `0x006540c4`). The ambient slot's source list is populated when
the camera enters a music region; the per-frame ducker
`FUN_00553660` (the music-manager tick — see §5) starts/stops the
node by calling `FUN_00553290` (which forwards into the play
wrappers at `0x0054f3c0` / `0x0054f400`).

The engine debug overlay confirms three concurrent ambient streams
exist at runtime: see the `"music [%s]…"`, `"ambient [%s]…"`,
`"interface [%s]…"` printf lines at `0x00612b7c`, `0x00612b58`,
and the inline literal in `FUN_004abc80` (`0x004abc80`).

Per-object `SBAmbientSound` entries (a tree creaking, a forge
hammering near a smith, etc.) flow through the same `SMSoundNode`
list — each object that owns the bit registers a positional sound
when its world cell becomes visible, and the listener-update
(`FUN_00547ef0`) supplies the position so FMOD's 3D mixer
attenuates it. Per-object registration call site is reached from
the per-frame entity-walker that AGENTS notes still as untraced;
the *observable* contract (named entry in `nsound.dat`, 3D
attenuation via `FSOUND_3D_SetAttributes`, ducked vs music) is
visible in the wrapper functions cited above.

## 4. One-shot SFX

The master engine-side dispatch is
`CDivSoundManager::PlaySound` at **`div.exe:0x00548ad0`**. Signature:

```c
int PlaySound(int *self, int category, float key, int param4);
```

Body:

1. Look up the `SfxClass` record for `key` via `FUN_0054fbd0`
   (probes the cache built from `nsound.dat` — see `sound.md`).
2. Pick a variant via `FUN_005489c0` (uniform random or weighted CDF
   per `type_tag`).
3. If 3D: load sample via cached path, then dispatch to
   `FUN_0054fa60` → `FSOUND_PlaySoundEx` + `FSOUND_3D_SetAttributes`.
4. If 2D / interface: same path but skipping the 3D attribute call.
5. If `type_tag == 2` (streaming): goes through
   `FSOUND_Stream_Play` instead of `PlaySoundEx`.

`PlaySound` is called from **31 sites** across the engine —
combat hit (`FUN_00451eb0`, `FUN_00451f90`), step / footstep paths
(`FUN_004d4140`, `FUN_004d6fc0`), spell cast (`FUN_004ef440`,
`FUN_00541420`), pickup (`FUN_00574440`, `FUN_00576b20`), UI
button (`FUN_00585900`), and the script-bridge entry
`FUN_00563910` / `FUN_005639f0` reachable from Osiris's
`PlaySoundObject` opcode (registered at `0x005387f0` via the
`AddSoundObject` family in `FUN_0053a550`). The Osiris dispatch
chain is `osiris VM → FUN_0053a550-registered handler → engine
PlaySound → FUN_0054fa60 → FSOUND_PlaySoundEx`.

Worked example — gore-decal hit:

```text
combat_resolve  (FUN_00451eb0)
  → CDivSoundManager::PlaySound(0x00548ad0,
        cat=hit_class, key=weapon.hit_sfx_key, ...)
  → SfxClass lookup via FUN_0054fbd0
  → variant pick via FUN_005489c0
  → FUN_0054fa60(channel, sample, sfxclass, variantIdx)
  → _FSOUND_PlaySoundEx(channel=-1, sample, dsp=0, paused=1)
  → _FSOUND_SetVolume(channel, computed_volume)
  → _FSOUND_SetPaused(channel, 0)            // start playback
```

Footstep specifically: the agent step path
`FUN_004d4140`/`FUN_004d6fc0` calls `PlaySound` with the
floor-material's footstep key — which `nsound.dat` resolves to a
weighted-variant `SfxClass` so each step picks a different WAV.

## 5. Music

Music plays through FMOD streams (`FSOUND_Stream_OpenFile` /
`FSOUND_Stream_Play`), not samples. Path prefix is hard-coded
to `wav\music\` at `0x0061c864` (`FUN_00550c90` / `FUN_00550fb0` /
`FUN_005510c0`). Open mode is `0x2130`.

The per-tick music driver is **`FUN_00553660`**
(`SMMusicManager::Update`, `.\SOUND\SMMusicManager.cpp`),
called from `FUN_00547ca0` (the per-tick sound update) which is
itself called from the simulation tick loop `FUN_00505bc0`.

`FUN_00553660` processes a state machine on
`mgr + 0x124` (event flags, bits `0x01 / 0x02 / 0x04 / 0x08 / 0x10
/ 0x20`):

- `0x01` — fade in current track.
- `0x02` — duck/mute (e.g. dialog start).
- `0x04` — restore from duck.
- `0x08` — bypass (no transition this tick).
- `0x10` / `0x20` — queued track change.

A track change calls `FUN_00553a30(mode)` which selects a
fade-class via the music handles at `mgr + 0xfc..0x10c` and
forwards into `FUN_00553290` to start the new stream and stop the
old. After a "no current music" interval (`mgr + 0xa0 == 0`),
`FUN_00553c70(60000)` schedules a 60s silence; if the music slot
is dirty (`mgr + 0xbc != 0`) `FUN_00553c20(60000 or 2000)` queues a
shorter fade.

Region binding is resolved at region-enter time: the player's
current region label (e.g. `Aleroth`) is matched against the
section-3 region list (see `sound.md` §section 3) and the
region's L0 list is pushed into the music slot, L3/L4 into the
day/night ambient slot.

Cross-fade: there is no separate cross-fade primitive; the engine
runs two parallel streams during a transition by simultaneously
ducking the outgoing stream (mode `0x02`) and starting the new one
at full volume (mode `0x01`). The `(%.2f->%.2f)(%s)` debug fields
at `0x00612b7c` are `(curVol -> targetVol)(constant|fading)`, where
`fading` means the per-tick interpolator at
`SMSoundNode + 0x44/0x48/0x4c` is currently active.

## 6. Voiced dialog

The bridge from `DivDialogSystem.dll`'s `GetAnswerSoundName` to
audio playback is **`FUN_00472570`** (`div.exe:0x00472570`):

```c
void FUN_00472570(CDialogManager *self) {
    char name[1024];
    if (!self->dialog || !(name_ptr = GetAnswerSoundName(self->dialog)))
        return;
    if (NoVoiceFlag) {                              // (DAT_00658c04+0x550)
        snprintf(name, 0x400, "voice\\%s.", name_ptr);
    } else {
        snprintf(name, 0x400, "voice\\%s.wavf.", name_ptr);
        if (PlayVoice(name)) return;                // try wavf first
        snprintf(name, 0x400, "voice\\%s.", name_ptr);
    }
    PlayVoice(name);
}
```

`GetAnswerSoundName` is the imported `?GetAnswerSoundName@CDivDialogSystem@@QAEPADXZ`
(IAT slot `EXTERNAL:0x89`, mangled label visible in the decompiled
listing as `_GetAnswerSoundName_CDivDialogSystem__QAEPADXZ`).

`PlayVoice` is **`FUN_0054aa90`** (`.\SOUND\playvoice.cpp`,
appended-suffix at `PTR_DAT_0061c73c`). It opens the named entry
through the engine's `fopen` shim, which routes the path through
the localised voice archive opened during sound init by
`FUN_0054ad30`:

```c
FUN_0054ad30():
    if (lang_id == 0)
        sprintf(buf, "voice\\voice.cmp");
    else
        sprintf(buf, "localizations\\%s\\voice.cmp", lang_dir);
    voice_archive = FUN_004f60f0(buf);              // open .cmp container
```

So the resolved file is `voice\voice.cmp::voice\<name>.wavf`
(or `.wav` fallback). The `.cmp` archive uses the same Family A
container format as `sound.cmp` (see `formats/cmp.md`).

`FUN_00472570` is invoked from the dialog plate's "advance line"
handler (`FUN_0051d530` at `0x0051d530` and `FUN_0051d700` at
`0x0051d700` — `.\PLATE\dialogpl.cpp`).

## Open questions

1. **Per-object `SBAmbientSound` registration site** — the bit
   meaning is firmly documented (objects column 31, bit `0x1000`),
   and the per-frame *consumer* is the `mgr + 0xa4` ambient list
   inside `FUN_00553660`, but the *producer* — the function that
   walks visible cells and pushes an `SMSoundNode` for each
   `SBAmbientSound` object — is not yet pinned. Likely lives in
   the same family as the spatial-hash visibility walker
   (`FUN_0056d720` / `FUN_0056e430` — see `render-trace.md`).

2. **`SMSoundNode` field layout.** Offsets `+0x04 mute`,
   `+0x05 duck`, `+0x44 fading`, `+0x48 srcVol`, `+0x4c dstVol`
   are visible in the debug overlay's printf args, but the rest of
   the structure (path, FMOD channel handle, end-callback) is
   inferred from access patterns only.

3. **Reverb per-region application.** `FSOUND_Reverb_SetProperties`
   is called once per preset at startup, but the per-region preset
   *switch* triggered by region-cross is reached from a path I
   haven't isolated; likely shares the same hooks as the music
   region change.

4. **Music section-3 fields L1 / L2.** `formats/sound.md` lists
   L0=music, L3=day-ambient, L4=night-ambient as observed; L1 and
   L2 carry data in shipped files but their consumer in
   `FUN_00553660` is not yet clear. Candidates: combat-stinger and
   stinger-on-region-change.

5. **The custom `FSOUND_File_SetCallbacks` callbacks.** FMOD I/O
   is hooked so streams can be served from inside the `voice.cmp`
   / `sound.cmp` archives; the four-callback table installed at
   sound-init time (open/read/seek/close) hasn't been traced.
