package ship

import (
	"bytes"
	"os"
	"testing"
)

func TestDMQuotedNamesRoundTrip(t *testing.T) {
	for _, name := range []string{`Engineer [A]`, `A "quoted" name`, `back\slash [room]`, "Line\nTwo"} {
		got, e := dmUnquote(dmQuote(name))
		if e != nil || got != name {
			t.Fatalf("%q became %q: %v", name, got, e)
		}
		jobs, e := parseCrew(`list(list(name = ` + dmQuote(name) + `, outfit = /datum/outfit/job/assistant, slots = 1))`)
		if e != nil || jobs[0].Name != name {
			t.Fatalf("job name did not round-trip: %+v %v", jobs, e)
		}
	}
}
func TestCrewReopensAfterWindowsCheckout(t *testing.T) {
	c, env := authorEnvironment(t)
	p, e := NewProject(c, env, "windows_crew", "Windows Crew", 24, 24)
	if e != nil {
		t.Fatal(e)
	}
	crewTestTypes(p)
	jobs, _ := p.CrewJobs("ship")
	jobs[1].Equipment = map[string]string{"head": ""}
	if e = p.SetCrewJobs("ship", jobs); e != nil {
		t.Fatal(e)
	}
	if e = p.Save(); e != nil {
		t.Fatal(e)
	}
	_, code := p.crewPaths()
	b, _ := os.ReadFile(code)
	_ = os.WriteFile(code, bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n")), 0600)
	reopened, e := OpenProject(c, env, p.Hull)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.Modified() {
		t.Fatal("Windows line endings marked crew dirty")
	}
}
func TestGeneratedRegistrationPreservesManualEdits(t *testing.T) {
	c, env := authorEnvironment(t)
	p, e := NewProject(c, env, "manual_source", "Manual Source", 24, 24)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Save(); e != nil {
		t.Fatal(e)
	}
	path := p.outputPaths()[1]
	b, _ := os.ReadFile(path)
	changed := append(b, []byte("\n// Hand-maintained behavior\n")...)
	_ = os.WriteFile(path, changed, 0600)
	reopened, e := OpenProject(c, env, p.Hull)
	if e != nil {
		t.Fatal(e)
	}
	reopened.Settings.Description = "Updated description"
	if reopened.Save() == nil {
		t.Fatal("overwrote code absent from project metadata")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, changed) {
		t.Fatal("manual code changed")
	}
	warnings, e := SaveProjectsWithWarnings([]*Project{reopened})
	if e != nil || len(warnings) != 1 || warnings[0].Path != path {
		t.Fatalf("save with warning failed: %+v %v", warnings, e)
	}
	backup, e := os.ReadFile(warnings[0].Backup)
	if e != nil || !bytes.Equal(backup, changed) {
		t.Fatal("manual code backup missing", e)
	}
	if reopened.Modified() {
		t.Fatal("successful save remains dirty")
	}
	if warnings, e = SaveProjectsWithWarnings([]*Project{reopened}); e != nil || len(warnings) != 0 {
		t.Fatalf("repeat save warned again: %+v %v", warnings, e)
	}
}
