package dmi

import "strings"

func preservedChunk(kind string) bool {
	return len(kind) == 4 && (kind[3]&32 != 0 || kind == "sRGB" || kind == "gAMA" || kind == "cHRM" || kind == "iCCP")
}

// ConversionIssues reports source information that cannot survive pixel edits.
// An unchanged save still returns the original bytes, including these chunks.
func (i *Icon) ConversionIssues() []string {
	var issues []string
	if i.BitDepth == 16 {
		issues = append(issues, "Reduce 16-bit color channels to 8-bit channels.")
	}
	var unsupported []string
	seen := map[string]bool{}
	for _, c := range i.Chunks {
		if c.Kind != layerChunk && !preservedChunk(c.Kind) && !seen[c.Kind] {
			unsupported = append(unsupported, c.Kind)
			seen[c.Kind] = true
		}
	}
	if len(unsupported) > 0 {
		issues = append(issues, "Remove PNG data that cannot safely follow image edits: "+strings.Join(unsupported, ", ")+".")
	}
	return issues
}

func (i *Icon) ConvertForEditing() {
	i.BitDepth = 8
	var chunks []Chunk
	for _, c := range i.Chunks {
		if c.Kind == layerChunk || preservedChunk(c.Kind) {
			chunks = append(chunks, c)
		}
	}
	i.Chunks = chunks
	i.Changed = true
}
