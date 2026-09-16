package dmi

import (
	"fmt"
	"image"
	"strconv"
)

// SliceSheet reads cells left-to-right, top-to-bottom. directionRows accepts
// sheets ordered by direction then frame; DMI storage is always frame-major.
func SliceSheet(sheet *image.NRGBA, width, height, dirs int, directionRows, separateStates bool, name string) (*Icon, error) {
	if width < 1 || height < 1 || sheet.Rect.Dx()%width != 0 || sheet.Rect.Dy()%height != 0 {
		return nil, fmt.Errorf("the sheet must contain whole %d x %d cells", width, height)
	}
	if dirs != 1 && dirs != 4 && dirs != 8 {
		return nil, fmt.Errorf("choose 1, 4 or 8 directions")
	}
	i, err := New(width, height)
	if err != nil {
		return nil, err
	}
	cols, rows := sheet.Rect.Dx()/width, sheet.Rect.Dy()/height
	count := cols * rows
	if int64(count)*int64(width)*int64(height) > MaxPixels || count%dirs != 0 {
		return nil, fmt.Errorf("sheet cells must divide evenly into the chosen directions")
	}
	i.States = nil
	i.Columns = cols
	frames := count / dirs
	for n := 0; n < count; n++ {
		if n == 0 || separateStates && n%dirs == 0 {
			s := NewState(name, width, height)
			if separateStates {
				s.Name = fmt.Sprintf("%s_%d", name, n/dirs+1)
			}
			s.Set("dirs", strconv.Itoa(dirs))
			if !separateStates {
				s.Set("frames", strconv.Itoa(frames))
			}
			s.Cels = nil
			i.States = append(i.States, s)
		}
		index := n
		if directionRows && !separateStates {
			index = (n%dirs)*frames + n/dirs
		}
		x, y := index%cols*width, index/cols*height
		cel := CopyImage(sheet.SubImage(image.Rect(x, y, x+width, y+height)).(*image.NRGBA))
		s := i.States[len(i.States)-1]
		s.Cels = append(s.Cels, cel)
	}
	return i, i.Validate()
}
