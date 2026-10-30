package main

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
)

func TestMixrIconGenerationIsDeterministic(t *testing.T) {
	source := filepath.Join("..", "..", "pkg", "mixr", "assets", "mixr-icon.svg")
	doc, err := readSVG(source)
	if err != nil {
		t.Fatal(err)
	}

	first := generateTestICO(t, doc)
	second := generateTestICO(t, doc)
	if !bytes.Equal(first, second) {
		t.Fatal("identical source produced different ICO bytes")
	}
	if got := binary.LittleEndian.Uint16(first[4:6]); got != uint16(len(iconSizes)) {
		t.Fatalf("ICO frame count = %d, want %d", got, len(iconSizes))
	}
	if first[6] != 16 || first[7] != 16 {
		t.Fatalf("first ICO frame is %dx%d, want 16x16", first[6], first[7])
	}
	lastEntry := 6 + 16*(len(iconSizes)-1)
	if first[lastEntry] != 0 || first[lastEntry+1] != 0 {
		t.Fatalf("last ICO frame is not encoded as 256x256")
	}
}

func generateTestICO(t *testing.T, doc svgDocument) []byte {
	t.Helper()
	frames := make([][]byte, 0, len(iconSizes))
	for _, size := range iconSizes {
		frame, err := renderPNG(doc, size)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	ico, err := encodeICO(iconSizes, frames)
	if err != nil {
		t.Fatal(err)
	}
	return ico
}
