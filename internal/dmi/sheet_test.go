package dmi

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSliceSheetDirectionOrder(t *testing.T) {
	sheet := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for n := 0; n < 8; n++ {
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				sheet.SetNRGBA(n%4*2+x, n/4*2+y, color.NRGBA{R: uint8(n + 1), A: 255})
			}
		}
	}
	i, err := SliceSheet(sheet, 2, 2, 4, true, false, "walk")
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range []uint8{1, 3, 5, 7, 2, 4, 6, 8} {
		if got := i.States[0].Cels[n].NRGBAAt(0, 0).R; got != want {
			t.Fatalf("cel %d = %d, want %d", n, got, want)
		}
	}
	i, err = SliceSheet(sheet, 2, 2, 4, false, true, "walls")
	if err != nil || len(i.States) != 2 || i.States[1].Cels[0].NRGBAAt(0, 0).R != 5 {
		t.Fatal("separate state import failed", err)
	}
	if _, err = SliceSheet(sheet, 3, 2, 4, false, false, ""); err == nil {
		t.Fatal("partial cells accepted")
	}
}

func TestPNGConversionAndExclusiveExport(t *testing.T) {
	root := t.TempDir()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "source.png")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "converted.DMI")
	if err = doc.Save(output, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(data, b.Bytes()) {
		t.Fatal("PNG was not converted to DMI")
	}
	decoded, err := Decode(data)
	if err != nil || decoded.Width != 8 || len(decoded.States) != 1 {
		t.Fatal("invalid converted DMI", err)
	}
	if err = Export(output, []byte("bad")); err == nil {
		t.Fatal("export replaced existing document")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(after, data) {
		t.Fatal("failed export changed file")
	}
	if err = Export(filepath.Join(root, "new.png"), b.Bytes()); err != nil {
		t.Fatal(err)
	}
}
