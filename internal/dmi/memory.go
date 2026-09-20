package dmi

import "image"

// HistoryCost accounts for replaced pixel buffers and copied metadata. Shared
// immutable cels do not count against every stroke's history budget.
func HistoryCost(before, after *Icon) int64 {
	collect := func(i *Icon) (map[*image.NRGBA]bool, int64) {
		images := map[*image.NRGBA]bool{}
		var metadata int64
		for _, s := range i.States {
			metadata += int64(128 + len(s.Fields)*32 + len(s.Cels)*8)
			for _, c := range s.Cels {
				images[c] = true
			}
			for _, l := range s.Layers {
				metadata += int64(64 + len(l.Cels)*8)
				for _, c := range l.Cels {
					images[c] = true
				}
			}
		}
		return images, metadata
	}
	a, am := collect(before)
	b, bm := collect(after)
	total := am + bm
	for c := range a {
		if !b[c] {
			total += int64(len(c.Pix))
		}
	}
	for c := range b {
		if !a[c] {
			total += int64(len(c.Pix))
		}
	}
	return total
}
