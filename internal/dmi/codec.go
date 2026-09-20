package dmi

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"
)

const signature = "\x89PNG\r\n\x1a\n"
const maxMetadata = 16 << 20

func Open(path string) (*Icon, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 256<<20+1))
	if e != nil {
		return nil, e
	}
	if len(data) > 256<<20 {
		return nil, fmt.Errorf("file exceeds the 256 MiB editing limit")
	}
	return Decode(data)
}
func chunks(data []byte) ([]Chunk, error) {
	if len(data) < 8 || string(data[:8]) != signature {
		return nil, fmt.Errorf("not a PNG/DMI file")
	}
	var result []Chunk
	for pos := 8; pos < len(data); {
		if len(data)-pos < 12 {
			return nil, fmt.Errorf("truncated PNG chunk")
		}
		size := int64(binary.BigEndian.Uint32(data[pos:]))
		end := int64(pos) + 12 + size
		if end > int64(len(data)) {
			return nil, fmt.Errorf("truncated PNG data")
		}
		kind := string(data[pos+4 : pos+8])
		payload := data[pos+8 : int(end)-4]
		if crc32.ChecksumIEEE(data[pos+4:int(end)-4]) != binary.BigEndian.Uint32(data[int(end)-4:]) {
			return nil, fmt.Errorf("invalid PNG checksum for %s", kind)
		}
		result = append(result, Chunk{kind, payload})
		pos = int(end)
		if kind == "IEND" {
			if pos != len(data) {
				return nil, fmt.Errorf("unexpected data after PNG end")
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("missing PNG end")
}
func inflate(data []byte) ([]byte, error) {
	r, e := zlib.NewReader(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, maxMetadata+1))
	if e != nil {
		return nil, e
	}
	if len(b) > maxMetadata {
		return nil, fmt.Errorf("DMI metadata is too large")
	}
	return b, nil
}
func description(c Chunk) (string, bool, error) {
	if c.Kind != "zTXt" && c.Kind != "tEXt" && c.Kind != "iTXt" {
		return "", false, nil
	}
	key, rest, ok := bytes.Cut(c.Data, []byte{0})
	if !ok || string(key) != "Description" {
		return "", false, nil
	}
	if c.Kind == "tEXt" {
		return string(rest), true, nil
	}
	if c.Kind == "zTXt" {
		if len(rest) < 1 || rest[0] != 0 {
			return "", true, fmt.Errorf("invalid compressed metadata")
		}
		b, e := inflate(rest[1:])
		return string(b), true, e
	}
	if len(rest) < 2 || rest[1] != 0 || rest[0] > 1 {
		return "", true, fmt.Errorf("invalid international metadata")
	}
	compressed := rest[0] == 1
	_, rest, ok = bytes.Cut(rest[2:], []byte{0})
	if !ok {
		return "", true, fmt.Errorf("invalid metadata language")
	}
	_, rest, ok = bytes.Cut(rest, []byte{0})
	if !ok {
		return "", true, fmt.Errorf("invalid metadata keyword")
	}
	if compressed {
		b, e := inflate(rest)
		return string(b), true, e
	}
	return string(rest), true, nil
}
func Decode(data []byte) (*Icon, error) {
	if len(data) > 256<<20 {
		return nil, fmt.Errorf("file exceeds the 256 MiB editing limit")
	}
	cs, e := chunks(data)
	if e != nil {
		return nil, e
	}
	cfg, e := png.DecodeConfig(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, fmt.Errorf("PNG dimensions exceed the editing limit")
	}
	i := &Icon{Original: append([]byte(nil), data...), BitDepth: 8}
	meta := ""
	found := false
	for _, c := range cs {
		if c.Kind == "IHDR" {
			i.BitDepth = c.Data[8]
		}
		text, yes, err := description(c)
		if err != nil {
			return nil, err
		}
		if yes {
			if found {
				return nil, fmt.Errorf("multiple DMI descriptions")
			}
			meta = text
			found = true
			continue
		}
		if c.Kind != "IHDR" && c.Kind != "IDAT" && c.Kind != "IEND" && c.Kind != "PLTE" && c.Kind != "tRNS" {
			i.Chunks = append(i.Chunks, c)
		}
	}
	if found {
		if e = i.parseMetadata(meta); e != nil {
			return nil, e
		}
	} else {
		i.Width, i.Height = cfg.Width, cfg.Height
		i.States = []*State{{Name: "", Fields: []Field{{"dirs", "1"}, {"frames", "1"}}}}
	}
	if i.Width < 1 || i.Height < 1 || i.Width > 8192 || i.Height > 8192 || cfg.Width%i.Width != 0 || cfg.Height%i.Height != 0 {
		return nil, fmt.Errorf("sheet dimensions do not match DMI canvas")
	}
	i.Columns = cfg.Width / i.Width
	total := 0
	for _, s := range i.States {
		dirs, frames := s.Dirs(), s.Frames()
		if (dirs != 1 && dirs != 4 && dirs != 8) || frames < 1 || frames > 65536 {
			return nil, fmt.Errorf("invalid frame/direction count in %q", s.Name)
		}
		total += dirs * frames
		if total > (cfg.Width/i.Width)*(cfg.Height/i.Height) {
			return nil, fmt.Errorf("DMI metadata refers to images outside the sheet")
		}
	}
	src, e := png.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	idx := 0
	for _, s := range i.States {
		for n := 0; n < s.Dirs()*s.Frames(); n++ {
			x, y := (idx%i.Columns)*i.Width, (idx/i.Columns)*i.Height
			cel := image.NewNRGBA(image.Rect(0, 0, i.Width, i.Height))
			if raw, ok := src.(*image.NRGBA); ok {
				for cy := 0; cy < i.Height; cy++ {
					offset := raw.PixOffset(x, y+cy)
					copy(cel.Pix[cy*cel.Stride:], raw.Pix[offset:offset+i.Width*4])
				}
			} else {
				for cy := 0; cy < i.Height; cy++ {
					for cx := 0; cx < i.Width; cx++ {
						cel.Set(cx, cy, src.At(x+cx, y+cy))
					}
				}
			}
			s.Cels = append(s.Cels, cel)
			idx++
		}
	}
	for _, chunk := range i.Chunks {
		if chunk.Kind == layerChunk {
			if err := i.decodeLayers(chunk.Data); err != nil {
				return nil, fmt.Errorf("could not restore Voidworks layers: %w", err)
			}
		}
	}
	return i, i.Validate()
}
func unquote(v string) (string, error) {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return "", fmt.Errorf("state name must be quoted")
	}
	var b strings.Builder
	for n := 1; n < len(v)-1; n++ {
		if v[n] == '\\' {
			n++
			if n >= len(v)-1 {
				return "", fmt.Errorf("incomplete state-name escape")
			}
			if v[n] != '\\' && v[n] != '"' {
				b.WriteByte('\\')
			}
		}
		b.WriteByte(v[n])
	}
	return b.String(), nil
}
func quote(v string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "\"", "\\\"") + "\""
}
func (i *Icon) parseMetadata(meta string) error {
	if len(meta) > maxMetadata {
		return fmt.Errorf("DMI metadata is too large")
	}
	i.Width, i.Height = 32, 32
	var current *State
	begun, ended, version := false, false, false
	for _, raw := range strings.Split(meta, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if line == "# BEGIN DMI" {
			begun = true
			continue
		}
		if line == "# END DMI" {
			ended = true
			break
		}
		if !begun {
			return fmt.Errorf("missing DMI header")
		}
		if strings.HasPrefix(line, "#") {
			if current == nil {
				i.Fields = append(i.Fields, Field{"", raw})
			} else {
				current.Fields = append(current.Fields, Field{"", raw})
			}
			continue
		}
		key, v, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid metadata line %q", line)
		}
		key, v = strings.TrimSpace(key), strings.TrimSpace(v)
		switch key {
		case "version":
			if v != "4.0" {
				return fmt.Errorf("unsupported DMI version %q", v)
			}
			version = true
		case "width", "height":
			n, e := strconv.Atoi(v)
			if e != nil {
				return fmt.Errorf("invalid %s", key)
			}
			if key == "width" {
				i.Width = n
			} else {
				i.Height = n
			}
		case "state":
			name, e := unquote(v)
			if e != nil {
				return e
			}
			current = &State{Name: name}
			i.States = append(i.States, current)
		default:
			if current == nil {
				i.Fields = append(i.Fields, Field{key, v})
				continue
			}
			if key == "dirs" || key == "frames" {
				n, e := strconv.Atoi(v)
				if e != nil || n < 1 {
					return fmt.Errorf("invalid %s in state %q", key, current.Name)
				}
				for _, f := range current.Fields {
					if f.Key == key {
						return fmt.Errorf("duplicate %s in state %q", key, current.Name)
					}
				}
			}
			current.Fields = append(current.Fields, Field{key, v})
		}
	}
	if !begun || !ended || !version {
		return fmt.Errorf("incomplete DMI metadata")
	}
	return nil
}
func (i *Icon) Metadata() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# BEGIN DMI\nversion = 4.0\n\twidth = %d\n\theight = %d\n", i.Width, i.Height)
	write := func(fields []Field) {
		for _, f := range fields {
			if f.Key == "" {
				b.WriteString(f.Value + "\n")
			} else {
				fmt.Fprintf(&b, "\t%s = %s\n", f.Key, f.Value)
			}
		}
	}
	write(i.Fields)
	for _, s := range i.States {
		fmt.Fprintf(&b, "state = %s\n", quote(s.Name))
		write(s.Fields)
	}
	b.WriteString("# END DMI\n")
	return b.String()
}
func (i *Icon) Sheet() (*image.NRGBA, error) {
	if e := i.Validate(); e != nil {
		return nil, e
	}
	count := max(1, i.CelCount())
	cols := max(1, min(i.Columns, count))
	rows := (count + cols - 1) / cols
	if int64(cols*i.Width)*int64(rows*i.Height) > MaxPixels {
		return nil, fmt.Errorf("packed sheet exceeds editing limit")
	}
	sheet := image.NewNRGBA(image.Rect(0, 0, cols*i.Width, rows*i.Height))
	idx := 0
	for _, s := range i.States {
		for _, cel := range s.Cels {
			x, y := (idx%cols)*i.Width, (idx/cols)*i.Height
			for cy := 0; cy < i.Height; cy++ {
				copy(sheet.Pix[(y+cy)*sheet.Stride+x*4:], cel.Pix[cy*cel.Stride:cy*cel.Stride+i.Width*4])
			}
			idx++
		}
	}
	return sheet, nil
}
func writeChunk(w *bytes.Buffer, c Chunk) {
	_ = binary.Write(w, binary.BigEndian, uint32(len(c.Data)))
	w.WriteString(c.Kind)
	w.Write(c.Data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(c.Kind))
	crc.Write(c.Data)
	_ = binary.Write(w, binary.BigEndian, crc.Sum32())
}
func Encode(i *Icon) ([]byte, error) {
	if !i.Changed && len(i.Original) > 0 {
		return append([]byte(nil), i.Original...), nil
	}
	if issues := i.ConversionIssues(); len(issues) > 0 {
		return nil, fmt.Errorf("explicit conversion required before editing: %s", strings.Join(issues, " "))
	}
	sheet, e := i.Sheet()
	if e != nil {
		return nil, e
	}
	layers, e := i.encodeLayers()
	if e != nil {
		return nil, e
	}
	var encoded bytes.Buffer
	if e = png.Encode(&encoded, sheet); e != nil {
		return nil, e
	}
	cs, e := chunks(encoded.Bytes())
	if e != nil {
		return nil, e
	}
	var out, compressed bytes.Buffer
	out.WriteString(signature)
	z := zlib.NewWriter(&compressed)
	if _, e = z.Write([]byte(i.Metadata())); e != nil {
		return nil, e
	}
	if e = z.Close(); e != nil {
		return nil, e
	}
	for _, c := range cs {
		writeChunk(&out, c)
		if c.Kind == "IHDR" {
			if layers != nil {
				writeChunk(&out, Chunk{layerChunk, layers})
			}
			for _, extra := range i.Chunks {
				// PNG safe-to-copy chunks and colour-space information survive edits.
				if preservedChunk(extra.Kind) {
					writeChunk(&out, extra)
				}
			}
			writeChunk(&out, Chunk{"zTXt", append([]byte("Description\x00\x00"), compressed.Bytes()...)})
		}
	}
	return out.Bytes(), nil
}
