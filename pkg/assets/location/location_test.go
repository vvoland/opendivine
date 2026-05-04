// SPDX-License-Identifier: GPL-3.0-only

package location

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"grono.dev/opendivine/internal/testutils"
)

func TestRoundTrip(t *testing.T) {
	in := &File{
		Tag: SubTagStory,
		Records: []Record{
			{V0: 40, V1: 58416, V3: 0, Name: "stps_hero"},
			{V0: 8000, V1: 3728, V3: 0, Name: "stps_Joram"},
			{V0: 1, V1: 2, V3: 3, Name: ""},
		},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, in); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n got %+v\n want %+v", out, in)
	}
}

func TestBadMagic(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte("Not a Divinity file")))
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestRealFile decodes the shipped global/location.000 from
// $TEST_GAMEDATA_PATH and asserts header + record presence.
func TestRealFile(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	path := filepath.Join(gamedata, "global/location.000")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	parsed, err := Decode(f)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	t.Logf("tag=%s, %d records; first=%+v", parsed.Tag, len(parsed.Records), parsed.Records[0])
	if parsed.Tag != SubTagStory {
		t.Errorf("expected StoryV1.0, got %q", parsed.Tag)
	}
	if len(parsed.Records) == 0 {
		t.Fatal("zero records")
	}
}
