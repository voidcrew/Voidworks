package wssprite

import (
	"bytes"
	"encoding/json"
	"sdmm/internal/dmi"
	"testing"
)

func TestRecoveredDocumentRetainsBaselineAndRejectsDamagedSavedImage(t *testing.T) {
	i, _ := dmi.New(2, 2)
	saved, _ := dmi.Encode(i)
	i.States[0].Name = "recovered"
	current, _ := dmi.Encode(i)
	snapshot := draft{Version: 1, Path: "missing.dmi", Baseline: bytes.Repeat([]byte{7}, 32), Saved: saved, Current: current}
	data, _ := json.Marshal(snapshot)
	doc, err := decodeDraft(data)
	if err != nil || doc.Icon.States[0].Name != "recovered" || doc.Saved.States[0].Name != "" || !doc.Modified() || !bytes.Equal(doc.Baseline, snapshot.Baseline) {
		t.Fatal("recovery lost content or conflict protection", err)
	}
	snapshot.Saved = []byte("damaged")
	data, _ = json.Marshal(snapshot)
	if _, err := decodeDraft(data); err == nil {
		t.Fatal("accepted a damaged saved image in recovery")
	}
}
