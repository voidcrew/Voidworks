package shippreview

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestStopClearsQueuedRebuildAndSurvivesReopening(t *testing.T) {
	root, client := incrementalFixture(t)
	environment := filepath.Join(root, "selected.dme")
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	before := map[string][]byte{}
	for _, name := range []string{"manifest.json", "alpha.png", "beta.png"} {
		before[name], _ = os.ReadFile(filepath.Join(output, name))
	}
	write(t, filepath.Join(root, "maps/alpha.dmm"), "12 changed alpha")
	write(t, filepath.Join(root, "maps/beta.dmm"), "12 changed beta")
	write(t, filepath.Join(root, "hold-child"), "")
	client.Request(root, environment)
	var child int
	until(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(root, "child-pid"))
		child, _ = strconv.Atoi(string(data))
		return child > 0
	})
	write(t, filepath.Join(root, "maps/room.dmm"), "12 queued room")
	other := New(client.profile)
	other.RequestFull(root, environment)
	until(t, func() bool { return other.Status(root).Queued })
	other.Stop(root, environment)
	until(t, func() bool { return other.Status(root).Phase == "stopped" })
	if processAlive(child) {
		t.Fatal("Stop left the renderer child running")
	}
	stages, err := filepath.Glob(filepath.Join(filepath.Dir(output), ".voidworks-previews-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("Stop left unfinished project staging folders: %v %v", stages, err)
	}
	for name, data := range before {
		after, _ := os.ReadFile(filepath.Join(output, name))
		if !bytes.Equal(data, after) {
			t.Fatalf("Stop published a partial generation: %s", name)
		}
	}
	if err := os.Remove(filepath.Join(root, "hold-child")); err != nil {
		t.Fatal(err)
	}
	reopened := New(client.profile)
	if reopened.Status(root).Phase != "stopped" {
		t.Fatal("pause was lost after reopening")
	}
	queuePath := filepath.Join(client.folder(root), "request.json")
	queue, _ := os.ReadFile(queuePath)
	reopened.Request(root, environment)
	after, _ := os.ReadFile(queuePath)
	if !bytes.Equal(queue, after) || reopened.Status(root).Phase != "stopped" {
		t.Fatal("a save restarted stopped previews")
	}
	var request struct{ Force, Paused bool }
	if err := json.Unmarshal(queue, &request); err != nil || request.Force || !request.Paused {
		t.Fatalf("Stop did not clear the queued full rebuild: %s", queue)
	}
	write(t, filepath.Join(root, "renders"), "")
	reopened.Resume(root, environment)
	until(t, func() bool { return reopened.Status(root).Phase == "complete" })
	renders, _ := os.ReadFile(filepath.Join(root, "renders"))
	if !slices.Equal(strings.Fields(string(renders)), []string{"beta.dmm", "room.dmm"}) {
		t.Fatalf("Resume discarded completed work or rebuilt unrelated maps: %s", renders)
	}
	refreshPreviews(t, root, reopened, false)
}

func TestInterruptedGenerationReusesCompletedRenders(t *testing.T) {
	interruptedPreview(t, false)
}

func TestInterruptedFullRebuildReusesOnlyItsCompletedRenders(t *testing.T) {
	interruptedPreview(t, true)
}

func interruptedPreview(t *testing.T, full bool) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows process-tree interruption fixture")
	}
	root, client := incrementalFixture(t)
	if full {
		refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	}
	write(t, filepath.Join(root, "hold-child"), "")
	if full {
		client.RequestFull(root, filepath.Join(root, "selected.dme"))
	} else {
		client.Request(root, filepath.Join(root, "selected.dme"))
	}
	until(t, func() bool { _, err := os.Stat(filepath.Join(root, "child-pid")); return err == nil })
	status := New(client.profile).Status(root)
	if status.PID == 0 {
		t.Fatal("worker did not publish its PID")
	}
	cmd := exec.Command("taskkill.exe", "/PID", strconv.Itoa(status.PID), "/T", "/F")
	configureProcess(cmd)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("could not interrupt test worker: %s %v", data, err)
	}
	until(t, func() bool { return !processAlive(status.PID) })
	if err := os.Remove(filepath.Join(root, "hold-child")); err != nil {
		t.Fatal(err)
	}
	reopened := New(client.profile)
	if status := reopened.Status(root); status.Phase != "failed" || !strings.Contains(status.Message, "interrupted") {
		t.Fatal("interrupted worker was not detected", status)
	}
	refreshPreviews(t, root, reopened, false, "beta.dmm", "room.dmm", "room_blue.dmm")
}

func TestStopBeforeFirstRequest(t *testing.T) {
	root, client := fixture(t)
	client.Stop(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return client.Status(root).Phase == "stopped" })
	client.Resume(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return client.Status(root).Phase == "complete" })
}

func TestStopWorkerLeftByOlderEditor(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows legacy helper migration")
	}
	root, client := fixture(t)
	folder := client.folder(root)
	helper, err := workerPath(folder)
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(folder, "worker-legacy.py")
	// The old protocol owns the same locks and blocks in subprocess.run with
	// no stop polling. Keep this fixture small instead of embedding old source.
	write(t, legacy, `import sys, runpy, os, time, subprocess
from pathlib import Path
folder = Path(sys.argv[1])
w = runpy.run_path(sys.argv[4])
guard = w['lock'](folder / 'queue.lock')
guard.__enter__()
with w['lock'](folder / 'worker.lock'):
    w['write_json'](folder / 'request.json', {'revision': 1, 'root': sys.argv[2], 'environment': sys.argv[3], 'force': True})
    w['write_json'](folder / 'status.json', {'phase': 'running', 'pid': os.getpid(), 'updated': time.time()})
    guard.__exit__(None, None, None)
    subprocess.run([sys.executable, '-c', "import time; from pathlib import Path; Path('legacy-running').write_text('yes'); time.sleep(20)"], cwd=sys.argv[2])
    with w['lock'](folder / 'queue.lock'):
        pass
`)
	executable, args, env, err := client.previewCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, append(args, "-B", legacy, folder, root, filepath.Join(root, "selected.dme"), helper)...)
	cmd.Env = env
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	until(t, func() bool { _, err := os.Stat(filepath.Join(root, "legacy-running")); return err == nil })
	client.Stop(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool {
		status := client.Status(root)
		if status.Phase == "error" {
			t.Fatal(status.Message)
		}
		return status.Phase == "stopped"
	})
	if processAlive(cmd.Process.Pid) {
		t.Fatal("the old worker is still running")
	}
	client.Resume(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return client.Status(root).Phase == "complete" })
}
