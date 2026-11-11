// SPDX-License-Identifier: GPL-3.0-only

// Command gamedata-probe prints the detected Divine Divinity install path.
package main

import (
	"fmt"
	"log"
	"path/filepath"

	"grono.dev/opendivine/internal/game/gamedata"
)

func main() {
	log.SetFlags(0)

	dir, err := gamedata.Resolve("")
	if err != nil {
		log.Fatal(err)
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(dir)
}
