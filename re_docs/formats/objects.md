# `static\objects.000` — global object catalogue

A flat, header-less array of fixed 148-byte (`0x94`) records — one per
*object kind* the engine knows about. The array is parallel to the
matching `CPackedb.0c` / `CPackedi.0c` imagelist: entry `N` in
`objects.000` describes the same object whose default sprite is
imagelist entry `N`. The shipped install has **exactly 7208 entries**
in both files; the engine asserts that the imagelist isn't larger
than `objects.000` at load time.

All integers little-endian.

## Record layout (148 bytes)

Field offsets confirmed by reading the engine's CSV exporter at
`div.exe:0x00581520` (`FUN_00581520`), which writes every column
verbatim from the in-memory record.

```text
struct Object {                       // 0x94 = 148 bytes
    u32   flags_a;                    // +0x00 — bit field for s_*  (slot, key, door, lever,
                                      //   disappears, chest, light, transforms, wt_use, value,
                                      //   function, function_parameter, move, item_class,
                                      //   can_carry, walk_through, inventory, strength).
                                      //   The bit indices live in DAT_00655970[]; the matching
                                      //   *_value is fetched via FUN_005918b0 from the
                                      //   16-byte payload at +0x04.
    u8    sub_values[16];             // +0x04 — packed value pool referenced by the s_* bits
    u32   weight;                     // +0x14 — Weight column
    i32   animation_index;            // +0x18 — index into APacked anim sets; -1 = no anim
    u32   sb_flags;                   // +0x1c — 22-bit bitfield (sb_sleep, sb_transparent,
                                      //   sb_shadow, sb_use_class, sb_real_black,
                                      //   sb_force_backwall, sb_force_leftwall,
                                      //   sb_force_floor, sb_no_look_through, sb_twinkle,
                                      //   sb_bow, sb_direct_move, sb_ambient_sound,
                                      //   sb_light_blocker, sb_light_bridge,
                                      //   sb_dont_loop_animation, sb_make_floating,
                                      //   sb_walk_on, sb_additive, sb_need_perfect_match,
                                      //   sb_show_in_object_box, sb_put_on)
                                      //   Bit 0 = sb_sleep, bit 21 = sb_put_on, in CSV order.
    char  name[32];                   // +0x20 — NUL-padded ASCII; "Object %d" if blank
    u32   id;                         // +0x30 — written by loader to match file index
    u32   class_;                     // +0x34 — Class column
    u32   break_anim_index;           // +0x38 — BreakAnimationIndex
    char  clothing_code[16];          // +0x3c — variable-length string in struct (NUL-padded)
    u32   floating_image_index;       // +0x4c
    u32   floating_list_index;        // +0x50
    u32   floating_highlight_index;   // +0x54
    u32   floating_pressed_index;     // +0x58
    u32   floating_disabled_index;    // +0x5c
    u8    pad_60[8];                  // +0x60 — 8 bytes runtime-only state
    u32   weapon_animation;           // +0x68
    u32   trade_priority;             // +0x6c
    u32   floating_group;             // +0x70
    u8    pad_74[8];                  // +0x74
    u32   automap_entry;              // +0x7c
    i16   bridge_patch_x_offset;      // +0x80
    i16   bridge_patch_y_offset;      // +0x82
    i16   bridge_patch_x_size;        // +0x84
    i16   bridge_patch_y_size;        // +0x86
    u8    pad_88[12];                 // +0x88 — runtime-only (e.g. cached sprite anchor offsets)
};
```

## Static behaviour bits (`sb_flags` at +0x1c)

The export iterates 5 outer × 5 inner = 22 bits starting from `0x01`,
shifting left each iteration:

| Bit | Mask | Field |
|---:|---|---|
| 0 | `0x000001` | `sb_sleep` |
| 1 | `0x000002` | `sb_transparent` |
| 2 | `0x000004` | `sb_shadow` |
| 3 | `0x000008` | `sb_use_class` |
| 4 | `0x000010` | `sb_real_black` |
| 5 | `0x000020` | `sb_force_backwall` |
| 6 | `0x000040` | `sb_force_leftwall` |
| 7 | `0x000080` | `sb_force_floor` |
| 8 | `0x000100` | `sb_no_look_through` |
| 9 | `0x000200` | `sb_twinkle` |
| 10 | `0x000400` | `sb_bow` |
| 11 | `0x000800` | `sb_direct_move` |
| 12 | `0x001000` | `sb_ambient_sound` |
| 13 | `0x002000` | `sb_light_blocker` |
| 14 | `0x004000` | `sb_light_bridge` |
| 15 | `0x008000` | `sb_dont_loop_animation` |
| 16 | `0x010000` | `sb_make_floating` |
| 17 | `0x020000` | `sb_walk_on` |
| 18 | `0x040000` | `sb_additive` |
| 19 | `0x080000` | `sb_need_perfect_match` |
| 20 | `0x100000` | `sb_show_in_object_box` |
| 21 | `0x200000` | `sb_put_on` |

`sb_force_floor` (bit 7) is the marker for floor-tile objects — useful
when separating floor placement from regular object instances.

## State / value pairs (`flags_a` at +0x00 + `sub_values` at +0x04)

`flags_a` is a packed bitmap of 19 boolean state flags (`s_slot`,
`s_key`, `s_door`, `s_lever`, `s_disappears`, `s_chest`, `s_light`,
`s_transforms`, `s_wt_use`, `s_value`, `s_function`,
`s_function_parameter`, `s_move`, `s_item_class`, `s_can_carry`,
`s_walk_through`, `s_inventory`, `s_strength`). Each bit's specific
position lives in the global table `DAT_00655970` (a `-1`-terminated
list of bit indices, in CSV-column order).

When a state bit is set, the matching `*_value` (a u32 or u8 — varies
per slot) is encoded in `sub_values[]` starting at `+0x04` and read
back via `FUN_005918b0(record+0x04, flags_a, bit_index)`.

To map bit to value-byte: walk the bits set in `flags_a` from low to
high; each set bit consumes the next packed value field, sized
according to the slot's defined width. Reverse-engineering the exact
slot widths needs more work but the indices in `DAT_00655970` are
themselves loaded from a small table in the binary.

## Loader citations

```text
div.exe:0x00586550   FUN_00586550   open static\\objects.000, slurp count*0x94 bytes,
                                    populate per-object id and validation pass.
div.exe:0x00581520   FUN_00581520   per-record CSV exporter — the most direct field-offset
                                    reference; this doc is built from its sprintf format string.
div.exe:0x005918b0   FUN_005918b0   value-bit unpacker: given (sub_values, flags_a, bit_index),
                                    returns the matching *_value field.
div.exe:0x00655970   DAT_00655970   global table of bit indices into flags_a, terminated by -1
                                    — defines the s_*/value column ordering.
```

Source path leak: `.\WORLD\objects.cpp`.

## Status

- 0x94-byte stride ✅
- Major fields ✅ (name, id, animation_index, sb_flags, weight, class,
  bridge-patch geometry, the floating-* indexed icons).
- 22 sb_* bits enumerated ✅ in CSV order (this doc's table).
- 19 s_*/value packing 🟡 — the bit indices are in `DAT_00655970`
  but per-slot value widths still TBD.
- Trailing 12 bytes at +0x88 ❓ — likely runtime-only state populated
  by the loader (cached sprite anchor or grid cell offsets).
