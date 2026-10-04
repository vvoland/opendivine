# Reverse-engineering notes

Notes on how Divine Divinity's `div.exe` (and its data files) work,
written while building OpenDivine. Each doc cites the binary by virtual
address (image base `0x00400000`) and, for file formats, verifies
against the shipped data.

## The engine in one paragraph

Divine Divinity is strongly **data-, event-, and script-driven**. The
executable is mostly an interpreter: object behaviour comes from
`objects.000` flags, spells from magic-script, loot from treasure
tables, monsters from eggs, item bonuses from affix tables, and balance
from `props.000`. Actions flow through an **agent message switch**
(`FUN_00509f10`) and an **Osiris event manager** (`[0x7447dc]`) rather
than direct calls. So when a code trace bottoms out at "it's
parameterised by data", the data file *is* the logic — reverse the
formats and the dispatch, not just inline code.

## Architecture & rendering

- [architecture.md](architecture.md) — binary overview: PE layout, the
  plugin renderer DLLs, subsystem DLLs (Osiris, dialog, FMOD).
- [render-trace.md](render-trace.md) — world render pipeline & depth sort.
- [renderer-plugins.md](renderer-plugins.md) — the swappable slash*.dll render backends + DllSlashed* ABI.
- [render-hero.md](render-hero.md) — hero sprite composition.
- [sound-runtime.md](sound-runtime.md) — audio runtime.
- [world-clock.md](world-clock.md) — game clock & day/night cycle.
- [lighting.md](lighting.md) — ambient/twilight + light sources + glow overlay.
- [screen-effects.md](screen-effects.md) — full-screen post-process transformations (poison/ice/ring).
- [sprite-builders.md](sprite-builders.md) — procedural particle-pattern builders (spiral/ring/pulse) for spell visuals.
- [weather.md](weather.md) — `CWeather` rain/storm + `CWeatherAction` lightning flashes.
- [cell-grid.md](cell-grid.md) — the world cell grid: the 8-byte cell record, its flag bits, `height.x<n>` and the query layer over it.
- [pathfinding.md](pathfinding.md) — A* navigation (half-cell grid, 8-dir).

## Core loop & dispatch

- [frame-loop.md](frame-loop.md) — `main` frame loop + the per-frame
  update order (`FUN_004ab4a0`): commands → clock → agents → projectiles
  → render. Where every subsystem ticks.

- [messages.md](messages.md) — the agent command bus: client/server
  request messages routed by `FUN_00509f10` to `Parse<Action>Message`
  handlers. The hub the interaction/combat docs reach through.

## Agents, combat & progression

- [agent.md](agent.md) — the `CAgent` instance struct (every living entity).
- [items.md](items.md) — `CItemStatistic` item property schema (damage
  dice, defense, slot/kind, magic, on-hit specials) + runtime instance.
- [stats.md](stats.md) — `CStats` attributes + the timed-boost system.
- [progression.md](progression.md) — XP award (kill reward, anti-grind
  cap, Wisdom boost) + the level mechanism.
- [combat.md](combat.md) — melee resolution flow (event-driven).
- [fight-controllers.md](fight-controllers.md) — the `CAgentFight`/`CClientFight`/`CPartyFight` attack-state controllers (client/server split).
- [death.md](death.md) — agent death sequence (`Die` handler, dead flag, post-death corpse callback).
- [party.md](party.md) — party, ownership & minions (summon/charm owner link, slave flag, `CPartyMember`).
- [npc-ai.md](npc-ai.md) — NPC behaviour as a data-driven `agentscript`
  language (125-command parser) + the per-frame behaviour tick.
- [animation.md](animation.md) — object/NPC animation state machine
  (`AnimationIndex` → `.cmp` frames, `Animan.cpp`, loop flag, 40fps advance).
- [skills-magic.md](skills-magic.md) — the skill tree + `CMagic*` effect
  classes (data-parameterised).
- [skill-tree.md](skill-tree.md) — the complete 96-skill roster (3 classes × 4 disciplines) from RTTI.
- [projectiles.md](projectiles.md) — arrow/bolt flight objects.
- [projectile-types.md](projectile-types.md) — the arrow/projectile/trajectory taxonomy (`CProjectile*`/`CArrow*`/`CPath*`) + Ranger arrows.
- [explosions.md](explosions.md) — area-of-effect blasts (fireball, shockwave).
- [painpoints.md](painpoints.md) — lingering area damage-over-time fields (cloud/elemental spells).
- [effects.md](effects.md) — `CAniEffect` world visual effects (hit flashes, spell/explosion FX) + attached effects.
- [monsters.md](monsters.md) — egg-based monster spawning.

## World objects & interaction

- [object-interaction.md](object-interaction.md) — `CObject::Use`: doors,
  chests, levers, locks, transforms, scripted use.
- [wall-elements.md](wall-elements.md) — wall/door/window classification.
- [minor-mechanics.md](minor-mechanics.md) — throwing, blood/gore decals, quick objects.
- [traps.md](traps.md) — the polymorphic `CTrapState` hierarchy.
- [teleporters.md](teleporters.md) — `CTeleportPlate`, the `dat\teleporter.txt` network.
- [monologues.md](monologues.md) — NPC barks (`monologue.cpp`), `mono.dat`/`mono.cmp`.
- [quest-log.md](quest-log.md) — the Diary/Journal quest tracker (`.\Diary\`, `diary_flags.txt`).
- [ai-behaviour.md](ai-behaviour.md) — native `CAgentBehavior`/`CGroupBehavior` AI (personalities + pack tactics).
- [alignment.md](alignment.md) — faction/alignment system (`alignment.dat`, the relation bit-matrix, friend/foe query).
- [script-commands.md](script-commands.md) — the internal script language.
- [script-language.md](script-language.md) — the `.\script\` AST VM (expressions, statements, if/while, variables) under mgcscrpt.

## UI

- [gui.md](gui.md) — BMGUI control toolkit + the plate (window) system.
- [floating-text.md](floating-text.md) — pop-up feedback text (damage numbers, XP gained) above entities.

## World & story

- [osiris.md](osiris.md) — story/quest engine (database + rules; story.000).
- [variables.md](variables.md) — script variables & state (global/agent/automatic scopes) shared by all script systems.
- [localization.md](localization.md) — `.\MISC\translator.cpp` localized text/voice (`localizations\<lang>\`).

- [dialogue.md](dialogue.md) — branching NPC conversations (DivDialogSystem.dll, dialogue flags + UTF-16 text).

## Items & economy

- [inventory.md](inventory.md) — item lists & equipment slots.
- [consumables.md](consumables.md) — potion/elixir use (`fcn.00587a20`): pool restore, timed boosts, cure.
- [clothing.md](clothing.md) — equipment → sprite composition (clothing codes).

## Data formats (`formats/`)

World & objects: [world](formats/world.md),
[objects](formats/objects.md), [collide](formats/collide.md),
[region](formats/region.md), [location](formats/location.md),
[automap](formats/automap.md).

Sprites & audio: [cpacked](formats/cpacked.md),
[imagelists](formats/imagelists.md),
[apacked](formats/apacked.md) (animation-class → frame index),
[cmp](formats/cmp.md), [sound](formats/sound.md),
[hufmann](formats/hufmann.md) (world/save Huffman codec).

Gameplay data: [eggs](formats/eggs.md) (spawns),
[treasure](formats/treasure.md) (loot tables),
[itemgen](formats/itemgen.md) (magic-item affixes),
[itemlink](formats/itemlink.md) (item name→type registry),
[props](formats/props.md) (balance/progression curves),
[magic](formats/magic.md) (spell params),
[skillcosts](formats/skillcosts.md) (per-rank skill-point costs),
[heroes](formats/heroes.md),
[savegame](formats/savegame.md), [misc](formats/misc.md),
[shroud](formats/shroud.md) (fog of war),
[config](formats/config.md) (settings & keys),
[musicdat](formats/musicdat.md) (music tracks/zones),
[reverbs](formats/reverbs.md) (EAX reverb),
[fnt](formats/fnt.md) (bitmap fonts),
[books](formats/books.md) (in-game book/note text),
[osi-static](formats/osi-static.md) (Osiris name/object snapshots).

## How these connect

A worn item's `ClothingCode` ([clothing](clothing.md)) drives the body
sprite; its [itemgen](formats/itemgen.md) suffix grants a
[`CMagic*`](skills-magic.md) effect whose per-level value lives in
[props](formats/props.md). A monster comes from an [egg](formats/eggs.md),
fights via [combat](combat.md) against [stats](stats.md), and drops from
a [treasure](formats/treasure.md) table. A door is a
[`CObject`](object-interaction.md) that a [trap](traps.md)
(`CTrapDoorState`) can also operate, sitting in a `door`
[wall element](wall-elements.md). Everything is a [`CAgent`](agent.md).

## Conventions

Functions are cited as `FUN_<va>` (Ghidra style). Status markers:
✅ confirmed, 🟡 partial / inferred, ❓ open.
