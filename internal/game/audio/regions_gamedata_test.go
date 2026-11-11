// SPDX-License-Identifier: GPL-3.0-only

package audio

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"grono.dev/opendivine/internal/testutils"
	"grono.dev/opendivine/pkg/assets/musicdat"
	"grono.dev/opendivine/pkg/assets/region"
)

// Check the cross-file join, not just each decoder in isolation: most shipped
// polygons have display names that differ from their music.dat identifiers.
func TestShippedMusicRegionBindings(t *testing.T) {
	t.Parallel()

	dir := testutils.TestGameData(t)
	data, err := os.ReadFile(filepath.Join(dir, "sound", "music.dat"))
	assert.NilError(t, err)
	md, err := musicdat.Decode(bytes.NewReader(data))
	assert.NilError(t, err)
	data, err = os.ReadFile(filepath.Join(dir, "global", "region.001"))
	assert.NilError(t, err)
	regions, err := region.Decode(bytes.NewReader(data))
	assert.NilError(t, err)
	r, err := NewRegionMusic(&MusicManager{zones: md.Zones}, regions)
	assert.NilError(t, err)

	bound := 0

	var unbound []string

	for _, rec := range regions.Records {
		if _, found := r.zones[musicZoneLabel(rec.Name)]; !found {
			unbound = append(unbound, rec.Name)
			continue
		}

		bound++
	}

	slices.Sort(unbound)
	assert.Equal(t, bound, 152)
	assert.Assert(t, is.DeepEqual(unbound, []string{"map3_extra1", "map3_extra2", "map3_extra3"}))
}
