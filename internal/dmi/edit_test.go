package dmi

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFrameEditsRetainDirectionsDelaysAndHistory(t *testing.T) {
	d, _ := Create(2, 2)
	_, _, e := d.Apply(func(i *Icon) error { return i.SetDirections(0, 4) })
	if e != nil {
		t.Fatal(e)
	}
	before, after, e := d.Apply(func(i *Icon) error { return i.InsertFrame(0, 0, true) })
	if e != nil {
		t.Fatal(e)
	}
	if before.States[0].Frames() != 1 || after.States[0].Frames() != 2 || len(after.States[0].Cels) != 8 {
		t.Fatal("frame insertion corrupted history")
	}
	after.States[0].SetDelays([]float64{0.5, 3})
	_, _, e = d.Apply(func(i *Icon) error { return i.MoveFrame(0, 0, 1) })
	if e != nil || !reflect.DeepEqual(d.Icon.States[0].Delays(), []float64{3, 0.5}) {
		t.Fatal("timing did not move with frames", e)
	}
	d.Restore(before)
	if d.Icon.States[0].Frames() != 1 {
		t.Fatal("undo failed")
	}
	d.Restore(after)
	if d.Icon.States[0].Frames() != 2 {
		t.Fatal("redo failed")
	}
	_, _, e = d.Apply(func(i *Icon) error { return i.DeleteFrame(0, 1) })
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = d.Apply(func(i *Icon) error { return i.DeleteFrame(0, 0) })
	if e == nil || d.Icon.States[0].Frames() != 1 {
		t.Fatal("deleted final frame")
	}
}

func TestDuplicatePreservesNormalMovementPairs(t *testing.T) {
	i := testIcon(t)
	i.States[0].Name, i.States[1].Name = "walk", "walk"
	if err := i.DuplicateStates([]int{0, 1}); err != nil {
		t.Fatal(err)
	}
	if i.States[2].Name != "walk_2" || i.States[3].Name != "walk_2" || i.States[2].Movement() || !i.States[3].Movement() {
		t.Fatal("duplicating states separated the normal/movement pair")
	}
}
func TestSplitCombinePreservesPixels(t *testing.T) {
	i := testIcon(t)
	original := i.States[0].Clone()
	if e := i.SplitState(0, true); e != nil {
		t.Fatal(e)
	}
	if e := i.CombineStates([]int{2, 3, 4, 5, 6, 7, 8, 9}, true); e != nil {
		t.Fatal(e)
	}
	combined := i.States[len(i.States)-1]
	for n, c := range original.Cels {
		if !bytes.Equal(c.Pix, combined.Cels[n].Pix) {
			t.Fatal("direction mapping changed")
		}
	}
	if e := i.SplitState(0, false); e != nil {
		t.Fatal(e)
	}
	if e := i.CombineStates([]int{len(i.States) - 2, len(i.States) - 1}, false); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(i.States[len(i.States)-1].Delays(), original.Delays()) {
		t.Fatal("split/combine lost timing")
	}
}
func TestFillAndTransformsPreserveSource(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	original := CopyImage(src)
	filled := Fill(src, image.Pt(1, 1), color.NRGBA{G: 255, A: 255}, image.Rect(0, 0, 2, 2))
	if filled.NRGBAAt(0, 0) != src.NRGBAAt(0, 0) || filled.NRGBAAt(3, 3).A != 0 {
		t.Fatal("fill escaped its boundary")
	}
	flipped := TransformImage(src, FlipHorizontal)
	if flipped.NRGBAAt(3, 0) != src.NRGBAAt(0, 0) {
		t.Fatal("flip failed")
	}
	if !bytes.Equal(original.Pix, src.Pix) {
		t.Fatal("editing mutated history")
	}
}
func TestSaveConflictBackupAndFailure(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "art.dmi")
	backup := filepath.Join(folder, "backups")
	d, _ := Create(2, 2)
	if e := d.Save(path, backup); e != nil {
		t.Fatal(e)
	}
	original, _ := os.ReadFile(path)
	if d.Modified() {
		t.Fatal("successful save is dirty")
	}
	before, _, e := d.Apply(func(i *Icon) error { i.States[0].Name = "edited"; return nil })
	if e != nil {
		t.Fatal(e)
	}
	if e = d.Save(path, backup); e != nil {
		t.Fatal(e)
	}
	restored, e := os.ReadFile(d.Backup)
	if e != nil || !bytes.Equal(restored, original) {
		t.Fatal("backup is not exact", e)
	}
	d.Restore(before)
	if !d.Modified() {
		t.Fatal("undo across save did not become dirty")
	}
	if e = os.WriteFile(path, []byte("external edit"), 0600); e != nil {
		t.Fatal(e)
	}
	if !d.ExternalChange() {
		t.Fatal("external edit not detected")
	}
	if e = d.Save(path, backup); e == nil {
		t.Fatal("overwrote external edit")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "external edit" {
		t.Fatal("failed save changed original")
	}
	if e = d.Save(filepath.Join(folder, "missing", "art.dmi"), backup); e == nil || !d.Modified() {
		t.Fatal("failed save cleared dirty state")
	}
}
func TestRejectedEditLeavesDocumentUntouched(t *testing.T) {
	d, _ := Create(4, 4)
	before := d.Icon
	_, _, e := d.Apply(func(i *Icon) error { return i.SetDirections(0, 7) })
	if e == nil || d.Icon != before {
		t.Fatal("invalid transaction changed document")
	}
}
