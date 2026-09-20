package dmi

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestExplicitConversionPreservesOriginalUntilAccepted(t *testing.T) {
	for _, depth := range []int{8, 16} {
		var b bytes.Buffer
		if depth == 16 {
			img := image.NewNRGBA64(image.Rect(0, 0, 2, 2))
			img.SetNRGBA64(0, 0, color.NRGBA64{R: 12345, A: 65535})
			if err := png.Encode(&b, img); err != nil {
				t.Fatal(err)
			}
		} else {
			i, _ := New(2, 2)
			data, _ := Encode(i)
			cs, _ := chunks(data)
			b.WriteString(signature)
			for _, c := range cs {
				writeChunk(&b, c)
				if c.Kind == "IHDR" {
					writeChunk(&b, Chunk{"vpAG", []byte("private geometry")})
				}
			}
		}
		i, err := Decode(b.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if len(i.ConversionIssues()) == 0 {
			t.Fatal("missing conversion notice")
		}
		exact, err := Encode(i)
		if err != nil || !bytes.Equal(exact, b.Bytes()) {
			t.Fatal("no-op conversion lost source", err)
		}
		edited := i.Clone()
		edited.Changed = true
		if _, err := Encode(edited); err == nil {
			t.Fatal("silently converted source")
		}
		edited.ConvertForEditing()
		data, err := Encode(edited)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := Decode(data)
		if err != nil || len(reopened.ConversionIssues()) > 0 || !bytes.Equal(reopened.States[0].Cels[0].Pix, i.States[0].Cels[0].Pix) {
			t.Fatal("explicit conversion changed displayed pixels", err)
		}
		if len(i.ConversionIssues()) == 0 {
			t.Fatal("conversion changed undo history")
		}
	}
}
