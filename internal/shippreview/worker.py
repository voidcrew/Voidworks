"""Run a project's preview generator independently of the editor window."""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import runpy
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
import traceback


PREVIEW_DIRECTORY = "voidcrew/modules/ship_upgrades/previews"


def preview_name(name):
    # Names are single, portable filenames, never paths or Windows streams.
    return (isinstance(name, str) and name.lower().endswith(".png")
            and not any(c in name for c in '<>:"/\\|?*\0')
            and not any(ord(c) < 32 for c in name)
            and name == name.strip() and len(name) > 4)


def linked(path):
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode):
        return True
    if not getattr(info, "st_file_attributes", 0) & 0x400:
        return False
    # OneDrive's Cloud Files tags describe storage, not a path redirection.
    # Allow CLOUD through CLOUD_F; keep rejecting junctions, symlinks and
    # unknown reparse points, including older runtimes without tag metadata.
    tag = getattr(info, "st_reparse_tag", 0)
    return tag & ~0xF000 != 0x9000001A


def preview_output(root):
    root = root.resolve(strict=True)
    output = root
    for part in PREVIEW_DIRECTORY.split("/"):
        output = output / part
        if output.exists() or output.is_symlink():
            if linked(output) or not output.is_dir():
                raise RuntimeError(f"Preview directory must not contain links: {output}")
    if output.resolve() != root / PREVIEW_DIRECTORY:
        raise RuntimeError("Preview directory is outside the project")
    return output


def ordinary_file(path):
    try:
        return not linked(path) and stat.S_ISREG(path.lstat().st_mode)
    except FileNotFoundError:
        return False


def manifest_images(manifest):
    if not isinstance(manifest, dict) or any(not isinstance(manifest.get(group), dict) for group in ("hulls", "modules")):
        raise RuntimeError("The generator did not produce a valid preview manifest")
    images = {}

    def entry_images(entry):
        if not isinstance(entry, dict):
            raise RuntimeError("Invalid preview manifest entry")
        name = entry.get("png")
        if name is not None:
            if not preview_name(name):
                raise RuntimeError("Preview manifest contains an unsafe image filename")
            key = name.casefold()
            if key in images and images[key] != name:
                raise RuntimeError("Preview image names differ only by capitalization")
            images[key] = name
        themes = entry.get("themes", {})
        if not isinstance(themes, dict) or (name is None and not themes):
            raise RuntimeError("Preview manifest entry has no image")
        for theme in themes.values():
            entry_images(theme)

    for group in ("hulls", "modules"):
        for entry in manifest[group].values():
            entry_images(entry)
    return images


def checked_manifest(directory):
    path = directory / "manifest.json"
    if not ordinary_file(path):
        raise RuntimeError("A regular preview manifest is required before cleanup")
    images = manifest_images(read_json(path, None))
    for name in images.values():
        if not ordinary_file(directory / name):
            raise RuntimeError(f"Preview manifest image is missing or linked: {name}")
    return images


def file_hash(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def cleanup_plan(root):
    output = preview_output(root)
    images = checked_manifest(output)
    if not images:
        raise RuntimeError("No active ship previews found; cleanup is disabled")
    files = []
    for path in sorted(output.iterdir()):
        if preview_name(path.name) and path.name.casefold() not in images and ordinary_file(path):
            files.append({"name": path.name, "sha256": file_hash(path), "size": path.stat().st_size})
    return {"root": os.path.normcase(str(root.resolve())), "directory": str(output), "files": files}


def cleanup_candidates(output, images, previous, old_images, approved):
    # Only a known, unmodified image from our previous successful render is
    # eligible automatically. Untracked leftovers always need explicit review.
    candidates = {}
    records = previous.get("images", {})
    if not approved and isinstance(records, dict):
        for name, record in records.items():
            if isinstance(record, dict) and name.casefold() in old_images:
                candidates[name] = record.get("png_sha256")
    # An explicit cleanup is limited to the reviewed selection. Do not add
    # automatic candidates that appeared while that review was open.
    candidates.update(approved)
    if not images:
        return {}  # Never interpret an empty fleet as permission to erase it.
    return {name: digest for name, digest in candidates.items()
            if preview_name(name) and name.casefold() not in images
            and ordinary_file(output / name) and file_hash(output / name) == digest}


def backup_cleanup(root, folder, candidates):
    if not candidates:
        return None
    output = preview_output(root)
    backups = folder / "cleanup-backups"
    if backups.resolve().is_relative_to(root.resolve()):
        raise RuntimeError("Cleanup backups must be stored outside the game project")
    backups.mkdir(parents=True, exist_ok=True)
    backup = Path(tempfile.mkdtemp(prefix=time.strftime("%Y%m%d-%H%M%S-"), dir=backups))
    for name, digest in candidates.items():
        source = output / name
        if not ordinary_file(source) or file_hash(source) != digest:
            raise RuntimeError(f"Preview changed before backup: {name}. Review cleanup again.")
        shutil.copy2(source, backup / name)
        if file_hash(backup / name) != digest:
            raise RuntimeError(f"Preview backup verification failed: {name}")
    write_json(backup / "restore.json", {"directory": str(output), "files": candidates,
                                         "restore": "Copy these PNG files back to directory. Do not overwrite newer files."})
    # This directory is deliberately persistent, even on failure. No automatic
    # retention/deletion: users can recover every removed preview from here.
    return backup


def write_json(path, value):
    temp = path.with_name(path.name + "." + str(os.getpid()) + ".tmp")
    temp.write_text(json.dumps(value), encoding="utf-8")
    os.replace(temp, path)


def read_json(path, default):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return default


def cached_json(path):
    try:
        value = read_json(path, {})
        return value if isinstance(value, dict) else {}
    except (OSError, ValueError):
        return {}


class IncrementalPreviews:
    """Reuse map renders while the project generator rebuilds current geometry."""

    def __init__(self, settings, script, environment, output, folder, force, rebuild=None):
        self.settings = settings
        self.script = script
        self.environment = environment
        self.output = output
        self.cache_path = folder / "render-cache.json"
        self.previous = cached_json(self.cache_path)
        self.force = force
        self.rebuild = rebuild
        self.progress_dir = folder / "render-progress"
        self.progress_path = self.progress_dir / "index.json"
        self.progress = cached_json(self.progress_path)
        self.context = None
        self.images = {}
        self.sources = {}
        self.digests = {}
        self.rendered = 0
        self.reused = 0
        self.legacy = {}
        manifest = cached_json(output / "manifest.json")
        for group in ("hulls", "modules"):
            entries = manifest.get(group, {})
            if isinstance(entries, dict):
                for entry in entries.values():
                    self.add_legacy(entry)

    def add_legacy(self, entry):
        if not isinstance(entry, dict):
            return
        if isinstance(entry.get("png"), str):
            self.legacy[entry["png"]] = entry.get("src_md5")
        themes = entry.get("themes", {})
        if isinstance(themes, dict):
            for variant in themes.values():
                self.add_legacy(variant)

    @staticmethod
    def signature(path):
        data = path.read_bytes()
        info = path.stat()
        return hashlib.md5(data).hexdigest(), info.st_size, info.st_mtime_ns

    def source(self, path):
        path = Path(path).resolve()
        signature = self.signature(path)
        before = self.sources.setdefault(path, signature)
        if before != signature:
            raise RuntimeError(f"Map changed during preview generation: {path}. The queued save will retry it.")
        data = path.read_bytes().replace(b"\r\n", b"\n")
        digest = hashlib.md5(data).hexdigest()
        self.digests[path] = {digest, hashlib.md5(data.replace(b"\n", b"\r\n")).hexdigest()}
        return path, digest

    def context_matches(self, context):
        if not isinstance(context, dict):
            return False
        context = dict(context)
        if not isinstance(context.get("environment"), str):
            return False
        environment = Path(context.get("environment", ""))
        if environment.is_absolute():
            try:
                environment = environment.relative_to(self.script.resolve().parents[2])
            except ValueError:
                pass
        context["environment"] = os.path.normcase(str(environment))
        return context == self.context

    def record_matches(self, previous, record, source):
        return (isinstance(previous, dict)
                and isinstance(previous.get("src_md5"), str)
                and previous.get("src_md5") in self.digests[source]
                and previous.get("source") in (record["source"], os.path.normcase(str(source))))

    def validate_sources(self):
        for path in self.sources:
            self.source(path)

    def render_reason(self, name, previous, matching, legacy, same_context, target):
        if self.force:
            return "full rebuild requested"
        if self.previous and not same_context:
            return "preview tools or environment changed"
        if matching or legacy:
            if not target.exists() and not target.is_symlink():
                return "preview file missing"
            if not ordinary_file(target):
                return "preview file is linked or not a regular file"
            return "preview image changed on disk" if matching else "preview image is invalid"
        if previous or self.legacy.get(name):
            return "map content or source path changed"
        return "no saved preview matches this map"

    def install(self):
        original_render = self.settings.get("render")
        if not callable(original_render):
            return False
        self.original_render = original_render
        self.settings["render"] = self.render
        original_dmm = self.settings.get("Dmm")
        if callable(original_dmm):
            def load_map(path, *args, **kwargs):
                self.source(path)
                result = original_dmm(path, *args, **kwargs)
                self.source(path)
                return result
            self.settings["Dmm"] = load_map
        return True

    def render(self, tool, source, destination, temp, dmm):
        source, digest = self.source(source)
        if self.context is None:
            from PIL import Image
            self.context = {
                "version": 1,
                "generator": hashlib.sha256(self.script.read_bytes()).hexdigest(),
                "renderer": hashlib.sha256(Path(tool).read_bytes()).hexdigest(),
                "python": sys.version,
                "pillow": Image.__version__,
                "environment": os.path.normcase(os.path.relpath(self.environment.resolve(), self.script.resolve().parents[2])),
            }
        name = destination.name
        target = self.output / name
        if not preview_name(name):
            raise RuntimeError("Invalid preview filename")
        record = {"source": os.path.normcase(os.path.relpath(source, self.script.resolve().parents[2])), "src_md5": digest}
        previous_images = self.previous.get("images", {})
        previous = previous_images.get(name) if isinstance(previous_images, dict) else None
        same_context = self.context_matches(self.previous.get("context"))
        matching = (not self.force and same_context and self.record_matches(previous, record, source))
        # Existing committed previews already carry source hashes. Adopt those
        # on the first incremental run instead of needlessly rendering a fleet.
        legacy = (not self.force and (not self.previous or same_context)
                  and not self.record_matches(previous, record, source)
                  and isinstance(self.legacy.get(name), str)
                  and self.legacy.get(name) in self.digests[source])
        progress = self.progress.get(name, {})
        if not isinstance(progress, dict):
            progress = {}
        checkpoint = self.progress_dir / name
        reused = (self.context_matches(progress.get("context"))
                  and self.record_matches(progress, record, source)
                  and (not self.force or (self.rebuild and progress.get("rebuild") == self.rebuild))
                  and ordinary_file(checkpoint) and progress.get("png_sha256") == file_hash(checkpoint))
        if reused:
            shutil.copy2(checkpoint, destination)
        if not reused and (matching or legacy) and ordinary_file(target):
            image_hash = hashlib.sha256(target.read_bytes()).hexdigest()
            if matching:
                reused = previous.get("png_sha256") == image_hash
            elif legacy:
                from PIL import Image
                try:
                    with Image.open(target) as image:
                        image.verify()
                    reused = True
                except (OSError, ValueError, SyntaxError):
                    pass
            if reused:
                shutil.copy2(target, destination)
        if reused:
            self.reused += 1
        else:
            reason = self.render_reason(name, previous, matching, legacy, same_context, target)
            print(f"Rendering changed or missing preview: {name} ({reason})", flush=True)
            self.original_render(tool, source, destination, temp, dmm)
            self.source(source)
            self.rendered += 1
        record["png_sha256"] = hashlib.sha256(destination.read_bytes()).hexdigest()
        self.images[name] = record
        # Checkpoint each finished image outside the project. A stopped/crashed
        # fleet pass can resume without publishing an incomplete manifest.
        try:
            self.progress_dir.mkdir(parents=True, exist_ok=True)
            checkpoint = self.progress_dir / name
            updated = {**record, "context": self.context, "rebuild": self.rebuild}
            if self.progress.get(name) != updated or not ordinary_file(checkpoint) or file_hash(checkpoint) != record["png_sha256"]:
                temp_image = checkpoint.with_suffix(".tmp")
                shutil.copy2(destination, temp_image)
                os.replace(temp_image, checkpoint)
                self.progress[name] = updated
                write_json(self.progress_path, self.progress)
        except OSError as error:
            print(f"Could not checkpoint preview: {error}", flush=True)
        print(f"Preview progress: {self.rendered} rendered, {self.reused} reused ({name}).", flush=True)

    def save(self):
        try:
            write_json(self.cache_path, {"context": self.context, "images": self.images})
        except OSError as error:
            # Published previews remain usable if the optional cache cannot be saved.
            print(f"Could not save the preview cache: {error}", flush=True)


@contextlib.contextmanager
def lock(path, blocking=True):
    # Kernel-owned locks are released even if Python or the editor crashes.
    with path.open("a+b") as file:
        file.seek(0)
        if os.name == "nt":
            import msvcrt
            if path.stat().st_size == 0:
                file.write(b"\0")
                file.flush()
            while True:
                try:
                    file.seek(0)
                    msvcrt.locking(file.fileno(), msvcrt.LK_NBLCK, 1)
                    break
                except OSError:
                    if not blocking:
                        yield False
                        return
                    time.sleep(.05)
            try:
                yield True
            finally:
                file.seek(0)
                msvcrt.locking(file.fileno(), msvcrt.LK_UNLCK, 1)
        else:
            import fcntl
            try:
                fcntl.flock(file, fcntl.LOCK_EX | (0 if blocking else fcntl.LOCK_NB))
            except BlockingIOError:
                yield False
                return
            try:
                yield True
            finally:
                fcntl.flock(file, fcntl.LOCK_UN)


class PreviewStopped(Exception):
    pass


def cancelled(folder, revision):
    item = read_json(folder / "request.json", {})
    return item.get("paused", False) or revision <= item.get("cancelled_through", -1)


@contextlib.contextmanager
def publication(folder, revision):
    # Stop and publication cannot race. Once publishing starts, finish the short
    # transaction (including rollback) before accepting a stop request.
    with lock(folder / "queue.lock"):
        if cancelled(folder, revision):
            raise PreviewStopped()
        yield


def generate(root, environment, folder, force=False, approved=None, revision=0, rebuild=None, stage=None):
    print("Preview refresh: " + ("full rebuild requested" if force else "incremental; reuse unchanged images") + ".", flush=True)
    script = root / "tools/ship_previews/generate_ship_previews.py"
    # Run the project's maintained implementation, including its smoothing fixes.
    namespace = runpy.run_path(str(script))
    settings = namespace["main"].__globals__
    output = preview_output(root)
    output.parent.mkdir(parents=True, exist_ok=True)
    cache = IncrementalPreviews(settings, script, environment, output, folder, force, rebuild)
    try:
        old_images = checked_manifest(output)
    except (OSError, ValueError, RuntimeError):
        old_images = {}  # Damaged old metadata cannot authorize automatic cleanup.
    incremental = cache.install()
    if not os.environ.get("DMM_TOOLS"):
        tool = shutil.which("dmm-tools")
        if tool:
            os.environ["DMM_TOOLS"] = tool
    staging = (tempfile.TemporaryDirectory(prefix=".voidworks-previews-", dir=output.parent)
               if stage is None else contextlib.nullcontext(stage))
    with staging as temp:
        stage = Path(temp)
        settings["OUTPUT_DIR"] = stage
        settings["ENVIRONMENT"] = str(environment)
        namespace["main"]()
        with publication(folder, revision):
            publish(root, folder, stage, output, cache, old_images, approved)
            if incremental:
                cache.save()
    print("Purchase previews updated.", flush=True)
    print(f"Preview images: {cache.rendered} rendered, {cache.reused} reused.", flush=True)


def publish(root, folder, stage, output, cache, old_images, approved):
    cache.validate_sources()
    images = checked_manifest(stage)
    # Publish only named images. Extra renderer outputs are not ownership
    # evidence and must never authorize deletion of another file.
    files = [stage / name for name in sorted(images.values())] + [stage / "manifest.json"]
    output = preview_output(root)
    output.mkdir(parents=True, exist_ok=True)
    candidates = cleanup_candidates(output, images, cache.previous, old_images, approved or {})
    recovery = backup_cleanup(root, folder, candidates)
    backup = stage / "backup"
    backup.mkdir()
    replaced = []
    removed = []
    try:
        for source in files:
            preview_output(root)
            target = output / source.name
            existed = target.exists()
            if (existed or target.is_symlink()) and not ordinary_file(target):
                raise RuntimeError(f"Preview target is not a regular file: {target.name}")
            if existed and target.read_bytes() == source.read_bytes():
                continue
            if existed:
                shutil.copy2(target, backup / source.name)
            os.replace(source, target)
            replaced.append((target, existed))
        for name, digest in candidates.items():
            preview_output(root)
            target = output / name
            # A user edit, replacement or link created since review wins.
            if not ordinary_file(target) or file_hash(target) != digest:
                continue
            target.unlink()
            removed.append(name)
    except BaseException:
        for name in removed:
            # Never overwrite a file created since the removal.
            with (output / name).open("xb") as restored:
                restored.write((recovery / name).read_bytes())
        for target, existed in reversed(replaced):
            if existed:
                os.replace(backup / target.name, target)
            else:
                target.unlink(missing_ok=True)
        raise
    if recovery:
        print(f"Cleanup backups: {recovery}", flush=True)
    kept = len(set(approved or {}) - set(removed))
    print(f"Preview cleanup: {len(removed)} unused moved to backup, {kept} reviewed files kept.", flush=True)


def terminate_generation(process):
    if process.poll() is not None:
        return
    if os.name == "nt":
        # This is our live Popen child, never a PID recovered from a stale file.
        result = subprocess.run(["taskkill.exe", "/PID", str(process.pid), "/T", "/F"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                       creationflags=subprocess.CREATE_NO_WINDOW)
        if result.returncode != 0 and process.poll() is None:
            raise RuntimeError("Could not stop the preview renderer")
    else:
        os.killpg(process.pid, signal.SIGKILL)
    process.wait()


def stop_legacy_worker(folder, pid):
    # Older releases have no cancellation protocol and can outlive an update.
    # Verify the command line belongs to this project's versioned helper before
    # stopping its tree. The queue lock prevents its normal exit during this.
    if os.name != "nt":
        raise RuntimeError("Close the preview helper from the older editor, then stop previews again")
    import ctypes
    from ctypes import wintypes
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    handle = kernel.OpenProcess(0x1000, False, pid)
    if not handle:
        raise RuntimeError("Could not identify the older preview helper")
    try:
        query = subprocess.run(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
                                f"(Get-CimInstance Win32_Process -Filter 'ProcessId = {int(pid)}').CommandLine | ConvertTo-Json -Compress"],
                               capture_output=True, text=True, creationflags=subprocess.CREATE_NO_WINDOW, check=True)
        command = json.loads(query.stdout)
        shell = ctypes.WinDLL("shell32", use_last_error=True)
        shell.CommandLineToArgvW.argtypes = [wintypes.LPCWSTR, ctypes.POINTER(ctypes.c_int)]
        shell.CommandLineToArgvW.restype = ctypes.POINTER(wintypes.LPWSTR)
        count = ctypes.c_int()
        argv = shell.CommandLineToArgvW(command, ctypes.byref(count))
        if not argv:
            raise RuntimeError("Could not identify the older preview helper")
        try:
            args = [argv[i] for i in range(count.value)]
        finally:
            kernel.LocalFree.argtypes = [ctypes.c_void_p]
            kernel.LocalFree(argv)
        helpers = [i for i, arg in enumerate(args[:-1])
                   if Path(arg).parent == folder and Path(arg).name.startswith("worker-")
                   and Path(arg).suffix == ".py" and Path(args[i + 1]) == folder]
        if len(helpers) != 1:
            raise RuntimeError("The running process is not this project's preview helper")
        subprocess.run(["taskkill.exe", "/PID", str(pid), "/T", "/F"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                       creationflags=subprocess.CREATE_NO_WINDOW, check=True)
    finally:
        kernel.CloseHandle(handle)


def stop(folder):
    with lock(folder / "queue.lock"):
        queue = folder / "request.json"
        item = read_json(queue, {"revision": 0})
        write_json(queue, {**item, "paused": True, "cancelled_through": item["revision"],
                           "force": False, "cleanup": {}, "rebuild": None})
        status = read_json(folder / "status.json", {})
        with lock(folder / "worker.lock", blocking=False) as idle:
            if not idle and status.get("protocol") != 2:
                stop_legacy_worker(folder, status.get("pid", 0))
                idle = True
            write_json(folder / "status.json", {**status, "phase": "stopped" if idle else "stopping",
                                                "queued": False, "updated": time.time()})


def request(folder, root, environment, force=False, approved=None, resume=False):
    queue = folder / "request.json"
    state = folder / "status.json"
    # Queue changes and worker shutdown share a short lock, so a save arriving
    # as the current worker exits cannot be stranded. Multiple editors share it.
    guard = lock(folder / "queue.lock")
    guard.__enter__()
    worker = None
    try:
        previous = read_json(queue, {"revision": 0})
        if previous.get("paused") and not (resume or force or approved):
            status = read_json(state, {})
            status["updated"] = time.time()
            write_json(state, status)
            return  # Saves do not restart previews after an explicit stop.
        revision = previous["revision"] + 1
        write_json(queue, {**previous, "revision": revision, "root": str(root), "environment": str(environment),
                           "paused": False,
                           "rebuild": str(time.time_ns()) if force else previous.get("rebuild"),
                           "force": force or previous.get("force", False),
                           "cleanup": {**previous.get("cleanup", {}), **(approved or {})}})
        worker = lock(folder / "worker.lock", blocking=False)
        if not worker.__enter__():
            worker.__exit__(None, None, None)
            worker = None
            status = read_json(state, {})
            status["queued"] = True
            status["updated"] = time.time()
            write_json(state, status)
            return
        interrupted = read_json(state, {}).get("phase") == "running"
        active = read_json(folder / "active-request.json", {})
        if interrupted and active.get("force"):
            latest = read_json(queue, {})
            latest["force"] = True
            latest["rebuild"] = latest.get("rebuild") or active.get("rebuild")
            write_json(queue, latest)
        while True:
            item = read_json(queue, {})
            revision = item["revision"]
            # A full rebuild belongs to this pass. Later saves queue an ordinary
            # incremental pass unless another full rebuild is requested.
            write_json(queue, {**item, "force": False, "cleanup": {}})
            current = folder / "active-request.json"
            output = preview_output(Path(item["root"]))
            output.parent.mkdir(parents=True, exist_ok=True)
            staging = tempfile.TemporaryDirectory(prefix=".voidworks-previews-", dir=output.parent)
            item["stage"] = staging.name
            write_json(current, item)
            write_json(state, {"phase": "running", "queued": False, "pid": os.getpid(), "protocol": 2, "updated": time.time()})
            guard.__exit__(None, None, None)
            guard = None
            log_path = folder / "generation.log"
            with staging, log_path.open("w", encoding="utf-8") as log:
                flags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
                process = subprocess.Popen(
                    [sys.executable, "-B", "-u", __file__, "--run", str(folder)],
                    cwd=item["root"], stdout=log, stderr=log, creationflags=flags, start_new_session=os.name != "nt",
                )
                stopped = False
                while process.poll() is None:
                    with lock(folder / "queue.lock"):
                        if cancelled(folder, revision):
                            stopped = True
                            terminate_generation(process)
                    if not stopped:
                        time.sleep(.1)
            guard = lock(folder / "queue.lock")
            guard.__enter__()
            latest = read_json(queue, {})
            stopped = stopped or cancelled(folder, revision)
            if process.returncode != 0 and not stopped:
                latest["force"] = latest.get("force", False) or item.get("force", False)
                latest["cleanup"] = {**item.get("cleanup", {}), **latest.get("cleanup", {})}
                latest["rebuild"] = latest.get("rebuild") or item.get("rebuild")
                write_json(queue, latest)
            if not latest.get("paused") and latest["revision"] != revision:
                continue
            write_json(state, {"phase": "stopped" if stopped else ("complete" if process.returncode == 0 else "failed"),
                              "queued": False, "updated": time.time()})
            return
    finally:
        # Release the worker while still holding the queue lock.
        if worker is not None:
            worker.__exit__(None, None, None)
        if guard is not None:
            guard.__exit__(None, None, None)


if __name__ == "__main__":
    if sys.argv[1] == "--scan":
        print(json.dumps(cleanup_plan(Path(sys.argv[2]))))
    elif sys.argv[1] == "--run":
        folder = Path(sys.argv[2])
        item = read_json(folder / "active-request.json", {})
        try:
            generate(Path(item["root"]), Path(item["environment"]), folder, item.get("force", False), item.get("cleanup", {}), item["revision"], item.get("rebuild"), item.get("stage"))
        except PreviewStopped:
            print("Preview generation stopped. Completed renders are saved for resuming.", flush=True)
    elif "--stop" in sys.argv[4:]:
        stop(Path(sys.argv[1]))
    else:
        folder = Path(sys.argv[1])
        try:
            approved = {}
            if "--cleanup" in sys.argv[4:]:
                plan_path = Path(sys.argv[sys.argv.index("--cleanup") + 1])
                if (plan_path.parent.resolve() != folder.resolve()
                        or not plan_path.name.startswith("approved-cleanup-")
                        or not ordinary_file(plan_path)):
                    raise RuntimeError("Cleanup requires a review saved by the editor")
                plan = read_json(plan_path, {})
                root = Path(sys.argv[2]).resolve()
                if plan.get("root") != os.path.normcase(str(root)) or plan.get("directory") != str(preview_output(root)):
                    raise RuntimeError("Cleanup review belongs to a different project")
                approved = {entry["name"]: entry["sha256"] for entry in plan["files"]}
                plan_path.unlink()
            request(folder, Path(sys.argv[2]), Path(sys.argv[3]), "--force" in sys.argv[4:], approved, "--resume" in sys.argv[4:])
        except BaseException:
            with (folder / "generation.log").open("a", encoding="utf-8") as log:
                traceback.print_exc(file=log)
            write_json(folder / "status.json", {"phase": "failed", "queued": False, "updated": time.time()})
            raise
