package dmenv

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const previewFixture = `#ifdef CBT
/obj/thing
	name = "compiled"
#else
/obj/thing
	name = "editor"
#endif
`

func pollPreview(t *testing.T, d *Dme) (*Dme, error) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		if env, ready, err := d.PollPreviewEnvironment(); ready {
			return env, err
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("preview parse did not finish")
	return nil, nil
}

func TestPollPreviewEnvironmentSharesOneParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.dme")
	if err := os.WriteFile(path, []byte(previewFixture), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Objects["/obj/thing"].Vars.TextV("name", ""); got != "editor" {
		t.Fatalf("editor definitions used %q", got)
	}
	d.WarmPreviewEnvironment()
	polled, err := pollPreview(t, d)
	if err != nil {
		t.Fatal(err)
	}
	if got := polled.Objects["/obj/thing"].Vars.TextV("name", ""); got != "compiled" {
		t.Fatalf("compiled appearances used %q", got)
	}
	waited, err := d.PreviewEnvironment()
	if err != nil || waited != polled {
		t.Fatalf("parsed twice: %v %v", waited, err)
	}
}

func TestPreviewEnvironmentRetriesAfterFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.dme")
	d := &Dme{RootFile: path}
	if env, err := pollPreview(t, d); err == nil || env != nil {
		t.Fatalf("parsed a missing project: %v %v", env, err)
	}
	// Polling reports the failure without parsing the project every frame.
	if _, ready, err := d.PollPreviewEnvironment(); !ready || err == nil {
		t.Fatalf("failure was not kept for polling: %v %v", ready, err)
	}
	if err := os.WriteFile(path, []byte(previewFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PreviewEnvironment(); err == nil {
		t.Fatal("the failed parse was not reported to its waiter")
	}
	env, err := d.PreviewEnvironment()
	if err != nil || env == nil {
		t.Fatalf("failed parse was not retried: %v %v", env, err)
	}
}
