// SPDX-License-Identifier: GPL-3.0-only

package character

// Hero layer composition, port of the engine's NPC/Combine.cpp logic
// (FUN_00439b70 in div.exe).  Given an animation slot and the
// character's currently equipped items, this produces the up-to-five
// .key group names that name the per-variant layers to draw.
//
// The composer is intentionally close to the original switch ladder
// rather than refactored, it's easier to verify against the
// decompiled C, and the engine's mapping is genuinely arbitrary in
// places (e.g. cVar4='G' → variant-A 2nd-char='A', variant-B 2nd-char
// chosen from a per-helmet table).

// CharacterEquipment captures the slots the composer needs.  Each
// field holds a character of the equipped item's ClothingCode (the
// objects.000 +0x3c field, a purpose-built positional visual code such
// as "MGB0", NOT the human-readable catalogue Name), or 0 if the slot
// is empty.  The engine reads these in FUN_0043a5b0 from the worn
// items in equipment slots {1,2,3,4,7}; see re_docs/clothing.md.  The
// letters key into the per-variant action mappings, a non-empty Helmet
// flips variants B/D from "MGB0" / "MGD0" to a numbered variant like
// "M1B3" depending on the helmet's class letter.
type CharacterEquipment struct {
	Helmet     byte // local_2c[0] in the engine, empty for default
	HelmetSub  byte // local_2c[1]
	HelmetCls  byte // local_2c[2], drives variant-B/D 2nd-char mapping
	Torso      byte // local_14[0]
	Legs       byte // local_1c[0]
	Face       byte // local_c[0]
	Weapon     byte // local_24[0]
	WeaponSub  byte // local_24[1]
	WeaponCls  byte // local_24[2]
	WeaponHand byte // local_40 in the engine, 0/1/2 (none / one-hand / two-hand)
}

// composeLayerNames produces the .key group names the engine would
// build for the given (anim slot, equipment) tuple.  Returns up to 5
// names in (A, B, C, D, E) variant order; empty string means the
// layer is intentionally skipped (e.g. no helmet → no C layer).
//
// Direct port of FUN_00439b70's name-build switch.  A few quirks:
//
//   - cVar4 is the "action letter" derived from the anim slot.  For
//     variant A it usually goes straight in as the 2nd char, with a
//     few rewrites (Z→C, G→A, X→H, V→H).
//   - Variants B and D use a more complex 2nd-char table that depends
//     on cVar4 *and* on the helmet's class letter, the engine
//     special-cases cVar4 ∈ {G, H, J, Z}.
//   - Variants C and E only show up if the corresponding equipment
//     slot is non-empty.
func composeLayerNames(animSlot int, eq CharacterEquipment) [5]string {
	var names [5]string

	// Step 1, derive the action letter from the anim slot.
	// (Direct port of the param_2 switch in FUN_00439b70.)
	cVar4 := byte('A')
	hasHelmet := eq.Helmet != 0
	switch animSlot {
	case 0:
		cVar4 = 'B'
	case 1:
		cVar4 = 'A'
	case 2:
		// Default idle.  No helmet → 'Q'; otherwise inherit the
		// helmet's first character.
		if !hasHelmet {
			cVar4 = 'Q'
		} else {
			cVar4 = eq.Helmet
			// Helmet sub V/X = early return (engine returns from
			// FUN_00439b70 with no layer names emitted), we
			// model that as an empty slot for everything.
			if eq.HelmetSub == 'V' || eq.HelmetSub == 'X' {
				return names
			}
		}
	case 3:
		cVar4 = 'D'
	case 4:
		cVar4 = 'E'
	case 5:
		cVar4 = 'F'
	case 6:
		cVar4 = 'H'
		if hasHelmet {
			cVar4 = eq.Helmet
			if eq.HelmetSub == 'V' || eq.HelmetSub == 'X' {
				return names
			}
		}
	case 7:
		if eq.WeaponHand != 2 {
			return names
		}
		cVar4 = 'P'
	case 11:
		cVar4 = 'G'
	case 12:
		cVar4 = 'C'
	case 13:
		cVar4 = 'Z'
	case 16:
		if eq.WeaponHand == 2 {
			cVar4 = 'J'
		} else {
			if hasHelmet && (eq.HelmetCls == 'A' || eq.HelmetCls == 'N' || eq.HelmetCls == 'O') {
				return names
			}
			cVar4 = 'J'
		}
	case 17:
		if !hasHelmet {
			return names
		}
		switch eq.HelmetCls {
		case 'B', 'C', 'D':
			cVar4 = 'M'
		case 'F', 'G', 'M':
			cVar4 = 'K'
		default:
			return names
		}
	case 18:
		cVar4 = 'U'
	}

	// Step 2, assemble each layer's name.

	// Layer A (variant A, legs+cloak).  The 2nd-char rewrite is
	// applied directly: Z→C, G→A, X→H, V→H.
	a2 := cVar4
	switch cVar4 {
	case 'Z':
		a2 = 'C'
	case 'G':
		a2 = 'A'
	case 'X', 'V':
		a2 = 'H'
	}
	a4 := byte('0')
	if eq.Legs != 0 {
		a4 = eq.Legs
	}
	names[0] = string([]byte{'M', a2, 'A', a4})

	// Layer B (variant B, torso+arms).  2nd-char depends on
	// cVar4 AND helmet class.
	b2 := layerBD2ndChar(cVar4, eq, hasHelmet)
	b4 := byte('0')
	if eq.Torso != 0 {
		b4 = eq.Torso
	}
	names[1] = string([]byte{'M', b2, 'B', b4})

	// Layer C (variant C, helmet).  Only added when a helmet is
	// equipped and its class letter is in {B, C, D, F, G, M}
	// (engine's early-skip predicate).
	if hasHelmet {
		switch eq.HelmetCls {
		case 'B', 'C', 'D', 'F', 'G', 'M':
			c2 := cVar4
			if cVar4 == 'V' || cVar4 == 'X' {
				c2 = 'H'
			}
			names[2] = string([]byte{'M', c2, 'C', eq.HelmetCls})
		}
	}

	// Layer D (variant D, face/head).  Same 2nd-char table as B.
	d2 := layerBD2ndChar(cVar4, eq, hasHelmet)
	d4 := byte('0')
	if eq.Face != 0 {
		d4 = eq.Face
	}
	names[3] = string([]byte{'M', d2, 'D', d4})

	// Layer E (variant E, weapon).  Only when a weapon is held.
	if eq.Weapon != 0 {
		e2 := cVar4
		if cVar4 == 'V' || cVar4 == 'X' {
			e2 = 'H'
		}
		names[4] = string([]byte{'M', e2, 'E', eq.Weapon})
	}

	return names
}

// layerBD2ndChar reproduces the variant-B/D 2nd-char mapping from
// FUN_00439b70, for cVar4 in {G, H, J, Z} the engine picks a
// number/letter based on the helmet's class.  For any other cVar4
// it falls through to cVar4 unchanged.
func layerBD2ndChar(cVar4 byte, eq CharacterEquipment, hasHelmet bool) byte {
	switch cVar4 {
	case 'Z':
		return 'C'
	case 'G':
		if hasHelmet {
			switch eq.HelmetCls {
			case 'E', 'H', 'L':
				return '1'
			case 'N':
				return '2'
			case 'O':
				return '3'
			case 'I', 'J', 'K':
				return 'L'
			}
		}
		return cVar4
	case 'H':
		if hasHelmet {
			if eq.Helmet == 'O' {
				return '4'
			}
			switch eq.HelmetCls {
			case 'I', 'J', 'K':
				return 'N'
			case 'N':
				return 'V'
			case 'O':
				return 'X'
			}
		}
		return cVar4
	case 'J':
		if eq.WeaponHand == 2 || !hasHelmet {
			return 'J'
		}
		switch eq.HelmetCls {
		case 'E', 'H', 'L':
			return 'P'
		case 'I', 'J', 'K':
			return 'R'
		}
		return 'J'
	}
	return cVar4
}
