package dmi

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testIcon(t *testing.T) *Icon {
	t.Helper()
	i, e := New(3, 2)
	if e != nil {
		t.Fatal(e)
	}
	i.States[0].Name = "a\\b\"c"
	i.States[0].Set("hotspot", "1,2,1")
	i.States[0].Set("future_property", "keep this")
	if e = i.SetDirections(0, 8); e != nil {
		t.Fatal(e)
	}
	if e = i.InsertFrame(0, 0, true); e != nil {
		t.Fatal(e)
	}
	i.States[0].SetDelays([]float64{0.5, 2.75})
	i.States[0].Set("rewind", "1")
	i.States[0].Set("loop", "3")
	for n := range i.States[0].Cels {
		c := CopyImage(i.States[0].Cels[n])
		c.SetNRGBA(1, 1, color.NRGBA{uint8(n * 12), 20, 30, 128})
		c.SetNRGBA(2, 0, color.NRGBA{201, 12, 45, 0})
		i.States[0].Cels[n] = c
	}
	movement := i.States[0].Clone()
	movement.Set("movement", "1")
	i.States = append(i.States, movement)
	i.Columns = 3
	return i
}
func compareIcons(t *testing.T, a, b *Icon) {
	t.Helper()
	if a.Width != b.Width || a.Height != b.Height || len(a.States) != len(b.States) || !reflect.DeepEqual(a.Fields, b.Fields) {
		t.Fatal("document metadata changed")
	}
	for n, s := range a.States {
		other := b.States[n]
		if s.Name != other.Name || !reflect.DeepEqual(s.Fields, other.Fields) || len(s.Cels) != len(other.Cels) {
			t.Fatalf("state %d metadata changed: %+v / %+v", n, s.Fields, other.Fields)
		}
		for c := range s.Cels {
			if !bytes.Equal(s.Cels[c].Pix, other.Cels[c].Pix) {
				t.Fatalf("state %d cel %d pixels changed", n, c)
			}
		}
	}
}
func TestRoundTripMetadataAndTransparentPixels(t *testing.T) {
	i := testIcon(t)
	data, e := Encode(i)
	if e != nil {
		t.Fatal(e)
	}
	read, e := Decode(data)
	if e != nil {
		t.Fatal(e)
	}
	compareIcons(t, i, read)
	unchanged, e := Encode(read)
	if e != nil || !bytes.Equal(data, unchanged) {
		t.Fatal("unchanged save altered original bytes", e)
	}
	read.Changed = true
	data2, e := Encode(read)
	if e != nil {
		t.Fatal(e)
	}
	read2, e := Decode(data2)
	if e != nil {
		t.Fatal(e)
	}
	compareIcons(t, i, read2)
}
func TestMalformedMetadataDoesNotPanic(t *testing.T) {
	for _, metadata := range []string{"", "# BEGIN DMI\nversion = 4.0\nstate = x\n# END DMI", "# BEGIN DMI\nversion = 4.0\nstate = \"a\"\nframes = potato\n# END DMI", "# BEGIN DMI\nversion = 4.0\nstate = \"a\"\ndirs = 1\ndirs = 4\n# END DMI"} {
		i := &Icon{}
		if e := i.parseMetadata(metadata); e == nil {
			t.Fatalf("accepted %q", metadata)
		}
	}
	data, _ := Encode(testIcon(t))
	for _, n := range []int{0, 7, 8, 20, len(data) - 1} {
		if _, e := Decode(data[:n]); e == nil {
			t.Fatal("accepted truncated data")
		}
	}
	data[len(data)-5] ^= 1
	if _, e := Decode(data); e == nil {
		t.Fatal("accepted corrupt checksum")
	}
}
func TestRealIconCorpus(t *testing.T) {
	root := os.Getenv("DMI_TEST_ROOT")
	if root == "" {
		t.Skip("set DMI_TEST_ROOT for a read-only corpus round trip")
	}
	count := 0
	e := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".dmi") {
			return nil
		}
		t.Run(strings.TrimPrefix(path, root), func(t *testing.T) {
			i, e := Open(path)
			if e != nil {
				t.Fatal(e)
			}
			exact, e := Encode(i)
			if e != nil || !bytes.Equal(exact, i.Original) {
				t.Fatal("no-op save changed bytes", e)
			}
			i.Changed = true
			if len(i.ConversionIssues()) > 0 {
				i.ConvertForEditing()
			}
			data, e := Encode(i)
			if e != nil {
				t.Fatal(e)
			}
			other, e := Decode(data)
			if e != nil {
				t.Fatal(e)
			}
			compareIcons(t, i, other)
		})
		count++
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if count == 0 {
		t.Fatal("empty corpus")
	}
	t.Logf("round-tripped %d DMI files without writing to the source tree", count)
}
func FuzzDecode(f *testing.F) {
	i, _ := New(2, 2)
	data, _ := Encode(i)
	f.Add(data)
	f.Add([]byte(signature))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		_, _ = Decode(data)
	})
}
