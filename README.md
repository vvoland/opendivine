# OpenDivine

A Go reimplementation of **Divine Divinity** (Larian Studios, 2002).

It reimplements the engine, not the content: you bring a copy of the game and OpenDivine runs off it.
Divine Divinity is still sold by Larian on Steam and GOG; please buy a copy if you don't have one already.

## Why?

Divine Divinity is a great game and it deserves to be playable forever.
Reimplementing the engine in a portable language means it can run on modern systems and multiple platforms.

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
