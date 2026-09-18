package ship

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Git may check DM files out with CRLF. That does not change their definitions.
func sameDMSource(a, b []byte) bool {
	return bytes.Equal(bytes.ReplaceAll(a, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
}

func (p *Project) expectGenerated(path string, source []byte) {
	if p.generatedBefore == nil {
		p.generatedBefore = map[string][]byte{}
	}
	p.generatedBefore[path] = append([]byte{}, source...)
}
func (p *Project) checkGenerated(path string) error {
	expected, ok := p.generatedBefore[path]
	if ok && p.files[path].Existed && !sameDMSource(expected, p.files[path].Before) {
		if p.saveWarnings != nil {
			p.saveWarnings[path] = true
			return nil
		}
		return fmt.Errorf("%s contains code changes missing from its workshop project; save stopped to preserve those changes", path)
	}
	return nil
}

// DM escapes brackets to prevent interpolation. Go's string decoder rejects
// these escapes, so translate just those before using its normal decoder.
func dmUnquote(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '[' || next == ']' {
				b.WriteByte(next)
				i++
				continue
			}
			b.WriteByte(s[i])
			b.WriteByte(next)
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return strconv.Unquote(b.String())
}
