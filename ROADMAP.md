# Roadmap — implementing the full game

Every step to a complete OpenDivine, dependency-ordered. Each leaf is intended
to be one small, reviewable change; each cites the RE doc that specs it.

## 0. Simulation foundation

- [ ] World clock — [world-clock](re_docs/world-clock.md)
  - [ ] `CClock`: tick counter, hour/day derivation (2880 ticks/hour, 72 s)
  - [ ] hour-change notification consumers can subscribe to
  - [ ] debug readout (hour/day)
- [ ] Message bus — [messages](re_docs/messages.md)
  - [ ] typed command queue + per-tick dispatch in the engine's update order
  - [ ] route player input (move/use) through it
- [ ] Engine-faithful collision — [formats/collide](re_docs/formats/collide.md)
  - [x] walkability cell grid: cube rasterization (inclusive rect stamp, masks from the object flags word, blocker refcount) — the engine has no per-move geometric test
  - [x] cell-granular mover blocking by mask; doors re-rasterize on use
  - [x] `fcn.0056f3c0` walkability: DIR16 destination + corner/pass-through chains (cell-space test, `internal/game/collision`)
  - [x] the 80-unit climb gate in walkability, fed by the loaded height bytes
  - [x] the 16-direction greedy leg stepper for the player: discrete occupancy cell + interpolated sprite, blocked-leg re-aim slide (`internal/game/mover`)
  - [ ] extend the stepper to NPCs and the blocked-leg full stop
  - [ ] pin the player-path mask (steppers use 0x13; we add the closed-door bit so closed doors block, unverified against the engine)
- [ ] Pathfinding — [pathfinding](re_docs/pathfinding.md)
  - [ ] half-cell walkability grid from collide + height
  - [ ] A*: 8-dir, displacement-derived costs, 80-unit climb cap
  - [ ] click-to-walk uses the planner (replace straight-line + wall slide)
  - [ ] reach tests for interaction/combat use the walkability tester
- [ ] Height map — [formats/world](re_docs/formats/world.md), [cell-grid](re_docs/cell-grid.md)
  - [x] load `height.x<n>`; per-cell height query
  - [x] terrain feeds collision: `loadRegion` → `Grid.LoadHeights`, the base height per cell, the climb bit derived from it (140,170 cells in region 0) and the map's own blockers (78,547) — the only walls no object record supplies
  - [ ] give the rasterizer the walk-on cubes' height: `region.go` builds every `collision.Cube` with `ZHeight`/`BaseZ` at 0, so a cube's climb byte is always 0 in game and only terrain can trip the gate. The collide record's `ZHeight` (i16[4]) is decoded and could be passed; `BaseZ` is the open half — [render-trace](re_docs/render-trace.md) gives `elevation = collide[catId].z_height + (layer & 0x3ff)` for a placed object, and `layer` is already decoded (`objectInst.Layer`), but that formula belongs to `0x0059fee0`'s elevation query, and that `0x0056d720` feeds the same sum into cell+6 when it stamps is not stated anywhere
  - [ ] terrain elevation feeds rendering (`screen_Y = worldY − byte7·4`, [render-trace](re_docs/render-trace.md))

## 1. World completeness

- [ ] Roofs — [formats/roofs](re_docs/formats/roofs.md)
  - [x] `roofs.dat` decoded (format work done)
  - [ ] load + render roof overlays from `roofs.000`
  - [ ] depth-band cut lines (A/B lists) for correct sorting
  - [ ] Roof Mode: hide/fade the roof covering the player (footprint + `dh_house` test)
- [ ] Day/night lighting — [lighting](re_docs/lighting.md)
  - [ ] gradsmal ramp load; CTwilight cursor (night 0 / day 320 / two 121-step ramps)
  - [ ] screen tint application
  - [ ] weather-dim and underground-night overrides
- [ ] Regions — [formats/region](re_docs/formats/region.md)
  - [ ] load `region.00N` polygons; winding point-in-polygon query
  - [ ] player region tracking (events on enter/leave)
- [ ] Shroud (fog of war) — [formats/shroud](re_docs/formats/shroud.md)
  - [ ] load `shroud.x<n>`; reveal on region entry; render
- [ ] Audio — [formats/sound](re_docs/formats/sound.md), [sound-runtime](re_docs/sound-runtime.md)
  - [ ] music.dat section 3 parser (5 lists per region)
  - [ ] zone music: L0 pick, weighted random, region-driven
  - [ ] ambients: L3 day / L4 night by clock hour (night = h<5 or ≥23)
  - [ ] `nsound.dat` SFX bank; positional playback
  - [ ] EAX-style reverb regions (optional) — [formats/reverbs](re_docs/formats/reverbs.md)
- [ ] Minimap / automap — [formats/automap](re_docs/formats/automap.md)

## 2. The dynamic world (objects & items)

- [ ] Object instances — [formats/osi-static](re_docs/formats/osi-static.md)
  - [ ] load `objects.x<n>` (28-byte records) + `extfree.x<n>`
  - [ ] overlay instances on the static world (cell-entry handle bits 12..31)
  - [ ] instance flags drive state: `sb_closed`/`sb_locked`/`sb_invisible`/`sb_broken`
  - [ ] value-pool reads (key id, lock, transform target; MSB-first widths)
- [ ] Interaction v2 — [object-interaction](re_docs/object-interaction.md)
  - [x] doors/chests toggle open (basic, catalogue flags)
  - [ ] state from the *instance* record, persisted
  - [ ] locks: `sb_locked` + key-in-inventory unlock (`sb_key` value)
  - [ ] open/close sounds (slots 8/9)
  - [ ] levers → linked-target propagation
  - [ ] `sb_transforms`: use-A-on-B transform codes — `objects.dat` recipes
  - [ ] tile-based use-target resolution (replace sprite-rect click heuristic)
- [ ] Item instances — [items](re_docs/items.md)
  - [ ] load `items.000` (CItem entries, generated-stat blobs)
  - [ ] CItem runtime model (durability, identified, boosts, charms)
- [ ] Inventories — [inventory](re_docs/inventory.md)
  - [ ] TRamFile pairs (`inv.i*/b*`): containers + 40-byte slot records
  - [ ] pickup / drop (world object ↔ inventory record)
  - [ ] stacking + `SplitItem` semantics
- [ ] Books & signs — [formats/books](re_docs/formats/books.md)
  - [ ] `books.000` (or `books.txt`) load; reader UI; BookImagePlate illustrations

## 3. Character systems

- [ ] Stats — [stats](re_docs/stats.md)
  - [ ] CStats base/effective blocks; per-class coefficient derivation
  - [ ] timed-boost list (types 0..13 fold)
  - [ ] equipment fold: 11-slot CItem array, fixpoint recompute (slot-1 port)
- [ ] Equipment — [clothing](re_docs/clothing.md), [render-hero](re_docs/render-hero.md)
  - [ ] equip/unequip against the slot map (0 helmet / 2 torso / 3 weapon / 4 shield / 7 leggings)
  - [ ] sprite recompose from worn ClothingCodes (composer already ported)
  - [ ] two-handed weapon hides the shield layer
- [ ] Inventory & character UI — [gui](re_docs/gui.md)
  - [ ] inventory plate (grid from slot records, drag, equip)
  - [ ] status plate (attributes, +/− point spend)
- [ ] Consumables — [consumables](re_docs/consumables.md)
  - [ ] use handler: restore/cure/invisibility/boost cases, props magnitudes
- [ ] Progression — [progression](re_docs/progression.md)
  - [ ] XP award: level-diff scaling, anti-grind jackpot, Wisdom boost
  - [ ] `XPforLevel(L) = 1000(L³−L)/3`; level-up grants (5 attr + 1 skill, +bonus)
  - [ ] level-up side effects (refills, recompute, FX/sound)
- [ ] Skills — [skill-tree](re_docs/skill-tree.md), [formats/skillcosts](re_docs/formats/skillcosts.md)
  - [ ] CSkill registry (96 skills, static ids), learn/rank-up, wgiaa costs
  - [ ] skills menu + hotbar slotting — [skills-magic](re_docs/skills-magic.md)

## 4. NPCs & AI

- [ ] Agent model — [agent](re_docs/agent.md)
  - [ ] CAgent struct: position/cell, flags words, CStats link
  - [ ] cell-grid registration + queries — [cell-grid](re_docs/cell-grid.md)
- [ ] Spawning — [formats/eggs](re_docs/formats/eggs.md), [monsters](re_docs/monsters.md)
  - [ ] `eggs.000` load; agents from class templates (deterministic stats)
  - [ ] monstergen: region → group → weighted class distribution
- [ ] NPC rendering
  - [ ] hero-bank NPCs (same pipeline); monster sprite banks
  - [ ] animation slots per class (data.000 dir counts)
- [ ] Scripted behaviour — [npc-ai](re_docs/npc-ai.md)
  - [ ] agentscript parser (~70 commands) + per-tick runner
  - [ ] loop points, perception commands, fight-mode commands
- [ ] Native behaviours — [ai-behaviour](re_docs/ai-behaviour.md)
  - [ ] 17 personality ticks (perceive→move / move-only / action-only families)
  - [ ] 7 group tactics
- [ ] Perception & factions
  - [ ] sight/hearing vs `hidden`/non-targetable bits — [agent](re_docs/agent.md)
  - [ ] alignment matrix, friend/foe query, runtime changes — [alignment](re_docs/alignment.md)
  - [ ] attractors (aggro points) — [combat](re_docs/combat.md)
- [ ] Barks — [monologues](re_docs/monologues.md)
- [ ] Party & minions — [party](re_docs/party.md)

## 5. Combat

- [ ] Melee resolution — [combat](re_docs/combat.md)
  - [ ] to-hit: closed-form chance (offense/defense/level-diff/mastery), clamp 20–95
  - [ ] damage fold: 12-step recipe (class core, equipment dice + wear, keyword/skill procs, Shield Bash, Increased Damage, mastery switch, dodge)
  - [ ] defense side: hit-location roll, armorclass, durability wear
  - [ ] damage-shield absorption (SMagic type 0xe)
  - [ ] fight controllers: player request → resolve split — [fight-controllers](re_docs/fight-controllers.md)
- [ ] Ranged — [projectiles](re_docs/projectiles.md), [projectile-types](re_docs/projectile-types.md)
  - [ ] fire-time damage roll; projectile carries it
  - [ ] flight paths (linear first; helix/spline/homing later)
  - [ ] impact → target combat virtuals; arrow-skill variants
- [ ] On-hit procs — effect spawn per channel chance — [combat](re_docs/combat.md)
- [ ] Death — [death](re_docs/death.md)
  - [ ] die sequence, corpse, loot hand-off, XP award, Osiris event
- [ ] Combat feedback
  - [ ] floating text (rise 1 px/tick × 40; font-select colours) — [floating-text](re_docs/floating-text.md)
  - [ ] hit FX (additive impact effects) — [effects](re_docs/effects.md)

## 6. Magic & effects

- [ ] Effect engine — [skills-magic](re_docs/skills-magic.md)
  - [ ] the 83-id effect table (named): apply/update/remove per effect
  - [ ] `magic.cmp` phase durations (wind-up/onset/sustain/tail) — [formats/magic](re_docs/formats/magic.md)
  - [ ] props.000 curve parameterisation (damage/duration/chance) — [formats/props](re_docs/formats/props.md)
  - [ ] status-bit application (hidden/frozen/non-targetable …)
- [ ] mgcscrpt VM — [script-language](re_docs/script-language.md)
  - [ ] expression + statement interpreter (the CExpression/CStatement trees)
  - [ ] host bindings (Source*/Target*/Center*/Screen*)
  - [ ] `.mgc` programs from `flat.cmp`; sprite-list emitters — [sprite-builders](re_docs/sprite-builders.md)
- [ ] Casting
  - [ ] mana cost/recuperation, targeting modes, hotbar activation
  - [ ] summons (counts from props; owner links) — [party](re_docs/party.md)
- [ ] Area effects
  - [ ] painpoints (lingering damage fields) — [painpoints](re_docs/painpoints.md)
  - [ ] explosions — [explosions](re_docs/explosions.md)
  - [ ] screen effects (poison/ice post-process) — [screen-effects](re_docs/screen-effects.md)
  - [ ] CFireEng flame (DOOM-fire kernel, Strength buff) — [effects](re_docs/effects.md)

## 7. Story

- [ ] Osiris engine — [osiris](re_docs/osiris.md)
  - [ ] `story.000` loader (XOR-0xAD strings, symbol/function tables, 8 node kinds, adaptors/dbases, goals)
  - [ ] RETE match/fire cycle (events → joins → rule actions)
  - [ ] goal INIT/KB/EXIT lifecycle
  - [ ] DIV function bindings — incremental, quest-driven (1175 total)
- [ ] Dialogue — [dialogue](re_docs/dialogue.md)
  - [ ] `dialogs.000` node tree; condition-list gating; weighted question order
  - [ ] conversation UI; DIALOG_EVENT flag bridge to Osiris
  - [ ] localized text (`dialogtxt.dat`) + voice hooks — [localization](re_docs/localization.md)
- [ ] Journal — [quest-log](re_docs/quest-log.md)
  - [ ] diary flags → journal entries; bestiary; conversation log
- [ ] World triggers
  - [ ] teleporters — [teleporters](re_docs/teleporters.md)
  - [ ] named locations (script targets) — [formats/location](re_docs/formats/location.md)
  - [ ] no-magic zones — region lists

## 8. Persistence

- [ ] Save/load — [formats/savegame](re_docs/formats/savegame.md)
  - [ ] 25-block `data.000` reader (spec machine-validated byte-exact)
  - [ ] writer (round-trip the shipped template)
  - [ ] dynamic-file bundling: map copies + Huffman `group.c*` — [formats/hufmann](re_docs/formats/hufmann.md)
  - [ ] save/load UI in the menu

## 9. Economy & generation

- [ ] Trade — [items](re_docs/items.md)
  - [ ] trade UI; closed-form pricing (Trader's Tongue tiers, stack rounding)
  - [ ] merchant stock behaviours (identify-all, full durability)
- [ ] Item generation — [formats/itemgen](re_docs/formats/itemgen.md)
  - [ ] affix descriptions load; 7 target categories; CBE expression evaluator
  - [ ] generated CItemStatistics (`__G%d`)
- [ ] Treasure — [formats/treasure](re_docs/formats/treasure.md)
  - [ ] drop tables: kind selector + value roll; loot on death/chests

## 10. Polish & fidelity

- [ ] Traps — [traps](re_docs/traps.md)
- [ ] Weather (rain/storm/lightning) — [weather](re_docs/weather.md)
- [ ] GUI plates — incremental per plate — [gui](re_docs/gui.md)
  - [ ] `.bmg` layouts from `flat.cmp`; divevent handler dispatch
- [ ] FMV playback (intro/outro `.bik`) — [fmv](re_docs/fmv.md)
- [ ] Localization: text.cmp, `.fnt` fonts — [localization](re_docs/localization.md), [formats/fnt](re_docs/formats/fnt.md)
- [ ] Minor mechanics: camera defs, ambient birds — [minor-mechanics](re_docs/minor-mechanics.md)
- [ ] Rendering fidelity gaps — [render-trace](re_docs/render-trace.md)
  - [ ] gore back-emit pass (z=30000)
  - [ ] override-sort sprites (`+0x3c & 0x200`)
  - [ ] terrain elevation in the depth sort
- [ ] Animation timing: per-descriptor frame durations, directional sub-ranges — [animation](re_docs/animation.md)
