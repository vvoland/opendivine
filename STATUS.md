# Status — how playable is OpenDivine?

**Short answer: not yet playable as a game.**
You can walk a fully rendered world with an animated hero, open doors and chests, and listen to menu and region-based music — but there are no monsters, no items you can pick up, no stats, no story, and no saving.

This page tracks the *player experience*. The full engineering plan
with every implementation step is in [ROADMAP.md](ROADMAP.md); the
reverse-engineering knowledge base behind it is complete
([RE_STATUS.md](RE_STATUS.md)).

## Exploring the world

- [x] The whole world renders — terrain, decals, objects, with the
  original engine's depth sorting
- [x] Ambient animated world props — torches and candles
- [x] Walk around: click-to-walk and WASD, camera follow, zoom
- [x] Walls and furniture block you (the engine's cell-grid collision; opening a door really unblocks its cells)
- [ ] Walking around obstacles automatically (pathfinding)
- [ ] Roofs hide when you step inside a building
- [ ] Day/night cycle (light tint, clock)
- [ ] Weather
- [ ] Map / minimap

## Your character

- [x] The hero renders and animates (walk + idle, all six class/sex
  combinations)
- [ ] Equipment visible on the character
- [ ] Attributes, health/mana, level-ups, skills
- [ ] Character/status screens

## Items

- [ ] Items exist in the world and can be picked up
- [ ] Inventory
- [ ] Equipping weapons and armour
- [ ] Potions, scrolls, books
- [ ] Shops and trading

## Combat & monsters

- [ ] Monsters spawn
- [ ] Fighting (melee, ranged, damage, death)
- [ ] Magic and skills

## Story & people

- [ ] NPCs walk around and react
- [ ] Conversations
- [ ] Quests and the journal
- [ ] The main story progresses (Osiris script engine)

## Doors, chests & the interactive world

- [x] Doors and chests open and close (basic: no keys, sounds, or
  chest contents yet)
- [ ] Locks and keys, levers, teleporters, traps

## Sound & video

- [x] Main-menu music
- [x] Region-based in-world music with smooth fades and shared-track continuity
- [ ] Day/night ambient sound
- [ ] Sound effects
- [ ] Intro/outro movies

## Saving

- [ ] Save and load
