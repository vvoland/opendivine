// SPDX-License-Identifier: GPL-3.0-only

package cpacked

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"grono.dev/opendivine/internal/testutils"
)

// TestRealImagelist decompresses every entry of imagelist 0 (the
// main object sprite bank) from $TEST_GAMEDATA_PATH.  Skips when
// the env var is unset.
func TestRealImagelist(t *testing.T) {
	gamedata := testutils.TestGameData(t)
	idxPath := filepath.Join(gamedata, "static/imagelists/CPackedi.0c")
	blobPath := filepath.Join(gamedata, "static/imagelists/CPackedb.0c")
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read %s: %v", idxPath, err)
	}
	blobFile, err := os.Open(blobPath)
	if err != nil {
		t.Fatalf("open %s: %v", blobPath, err)
	}
	defer blobFile.Close()
	st, _ := blobFile.Stat()

	r, err := NewReader(idx, blobFile, st.Size())
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	t.Logf("imagelist: %d entries", r.Count())

	// Decompress every entry — proof the LZO pipeline works for the
	// whole shipped imagelist, not just the first few.
	for i := range r.Count() {
		payload, err := r.CellPayload(i)
		if err != nil {
			t.Fatalf("CellPayload(%d): %v", i, err)
		}
		if len(payload) == 0 {
			t.Fatalf("entry %d decompressed to zero bytes", i)
		}
	}
	t.Logf("decompressed all %d cells cleanly", r.Count())

	// Decode a sample of standard-flag cells fully into RGB565.
	tested := 0
	for i := 0; i < r.Count() && tested < 50; i++ {
		e, _ := r.Entry(i)
		if e.Flags != FlagStandard {
			continue
		}
		c, err := r.DecodeCell(i)
		if err != nil {
			t.Fatalf("DecodeCell(%d): %v", i, err)
		}
		if c.Width != int(e.Width) || c.Height != int(e.Height) {
			t.Errorf("entry %d dim mismatch", i)
		}
		if len(c.RGB565) != c.Width*c.Height*2 {
			t.Errorf("entry %d raster size %d, want %d", i, len(c.RGB565), c.Width*c.Height*2)
		}
		tested++
	}
	t.Logf("decoded %d standard-flag cells into RGB565", tested)
}

func TestBadIndexAlignment(t *testing.T) {
	idx := bytes.Repeat([]byte{0}, EntrySize-1)
	_, err := NewReader(idx, bytes.NewReader(nil), 0)
	if err == nil {
		t.Fatal("expected ErrIndexAlignment")
	}
}

func TestEmpty(t *testing.T) {
	r, err := NewReader(nil, bytes.NewReader(nil), 0)
	if err != nil {
		t.Fatalf("empty index: %v", err)
	}
	if r.Count() != 0 {
		t.Errorf("count = %d, want 0", r.Count())
	}
}
