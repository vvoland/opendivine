# OpenDivine

[![Matrix](https://img.shields.io/badge/matrix-%23opendivine-0dbd8b?logo=matrix)](https://matrix.to/#/!yvmXJIsXKstaHvttsE:matrix.org)

A Go reimplementation of **Divine Divinity** (Larian Studios, 2002).

> [!WARNING]
> OpenDivine is not fully playable yet. You can explore the world, but the full gameplay loop is still under development.

Renders the world and lets you walk around; combat, dialogue, items, and the rest of the gameplay loop are on the roadmap.

See [`STATUS.md`](STATUS.md) for the feature checklist.

It reimplements the engine, not the content: **you need a copy of the game** to run OpenDivine off it.

Divine Divinity is still sold by Larian on Steam and GOG. If you don't have one already, you can buy it:

- https://www.gog.com/en/game/divine_divinity
- https://store.steampowered.com/app/214170/Divine_Divinity/

## Why?

Divine Divinity is a great game and it deserves to be playable forever.

## Running it

Requires Go 1.24 and a copy of Divine Divinity installed on disk.

You can either install from source with Go or download a prebuilt binary from the project's [GitHub Releases](https://github.com/vvoland/opendivine/releases).

```sh
# Install
go install grono.dev/opendivine@latest

# Run
opendivine
```

OpenDivine looks for the install at `-gamedata <path>`, then `$OPENDIVINE_GAMEDATA`, `./gamedata`, and finally the standard Steam and GOG paths.

On failure it prints every path it tried.

Useful flags: `-class {surm,surf,warm,warf,wizm,wizf}`, `-posx`, `-posy`, `-zoom`, `-region`.

## Showcase


<img src="https://github.com/user-attachments/assets/65821801-59ae-41fb-85d5-7e1debde13e9" />

## Reverse engineering

The majority of the reverse engineering and assembly analysis was performed by Claude Opus 4.7 (Anthropic).

The copy of the game used was from Steam; `div.exe` SHA256 `cd9f7a07c7a605c21052b3517346c7a63083063fb491276a3e4c9836c9b42a67`.

## Goals

- Gameplay parity good enough to play the whole game.

## Non-goals

- Bug-for-bug parity where the engine bug is just a bug.
- Redistributing any of Larian's original assets.

## License

GPL-3.0-only.
Original game assets remain the property of Larian Studios.
