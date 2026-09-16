"""Run a project's preview generator independently of the editor window."""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import runpy
import shutil
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
    return (stat.S_ISLNK(info.st_mode)
            or bool(getattr(info, "st_file_attributes", 0) & 0x400))  # Windows reparse points.


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

    def __init__(self, settings, script, environment, output, folder, force):
        self.settings = settings
        self.script = script
        self.environment = environment
        self.output = output
        self.cache_path = folder / "render-cache.json"
        self.previous = cached_json(self.cache_path)
        self.force = force
        self.context = None
        self.images = {}
        self.sources = {}
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
        return path, signature[0]

    def validate_sources(self):
        for path in self.sources:
            self.source(path)

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
                "environment": os.path.normcase(str(self.environment.resolve())),
            }
        name = destination.name
        target = self.output / name
        record = {"source": os.path.normcase(str(source)), "src_md5": digest}
        previous_images = self.previous.get("images", {})
        previous = previous_images.get(name) if isinstance(previous_images, dict) else None
        matching = (not self.force and self.previous.get("context") == self.context
                    and isinstance(previous, dict)
                    and all(previous.get(key) == value for key, value in record.items()))
        # Existing committed previews already carry source hashes. Adopt those
        # on the first incremental run instead of needlessly rendering a fleet.
        legacy = (not self.force and not self.previous
                  and self.legacy.get(name) == digest)
        reused = False
        if (matching or legacy) and target.is_file():
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
            print(f"Rendering changed or missing preview: {name}", flush=True)
            self.original_render(tool, source, destination, temp, dmm)
            self.source(source)
            self.rendered += 1
        record["png_sha256"] = hashlib.sha256(destination.read_bytes()).hexdigest()
        self.images[name] = record

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


def generate(root, environment, folder, force=False, approved=None):
    script = root / "tools/ship_previews/generate_ship_previews.py"
    # Run the project's maintained implementation, including its smoothing fixes.
    namespace = runpy.run_path(str(script))
    settings = namespace["main"].__globals__
    output = preview_output(root)
    output.parent.mkdir(parents=True, exist_ok=True)
    cache = IncrementalPreviews(settings, script, environment, output, folder, force)
    try:
        old_images = checked_manifest(output)
    except (OSError, ValueError, RuntimeError):
        old_images = {}  # Damaged old metadata cannot authorize automatic cleanup.
    incremental = cache.install()
    if not os.environ.get("DMM_TOOLS"):
        tool = shutil.which("dmm-tools")
        if tool:
            os.environ["DMM_TOOLS"] = tool
    with tempfile.TemporaryDirectory(prefix=".voidworks-previews-", dir=output.parent) as temp:
        stage = Path(temp)
        settings["OUTPUT_DIR"] = stage
        settings["ENVIRONMENT"] = str(environment)
        namespace["main"]()
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
    if incremental:
        cache.save()
    print("Purchase previews updated.", flush=True)
    if recovery:
        print(f"Cleanup backups: {recovery}", flush=True)
    kept = len(set(approved or {}) - set(removed))
    summary = f"Preview images: {cache.rendered} rendered, {cache.reused} reused, {len(removed)} unused moved to backup."
    if kept:
        summary += f" {kept} reviewed files kept (changed, used, missing or unsafe)."
    print(summary, flush=True)


def request(folder, root, environment, force=False, approved=None):
    queue = folder / "request.json"
    state = folder / "status.json"
    # Queue changes and worker shutdown share a short lock, so a save arriving
    # as the current worker exits cannot be stranded. Multiple editors share it.
    guard = lock(folder / "queue.lock")
    guard.__enter__()
    worker = None
    try:
        previous = read_json(queue, {"revision": 0})
        revision = previous["revision"] + 1
        write_json(queue, {"revision": revision, "root": str(root), "environment": str(environment),
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
        while True:
            item = read_json(queue, {})
            revision = item["revision"]
            # A full rebuild belongs to this pass. Later saves queue an ordinary
            # incremental pass unless another full rebuild is requested.
            write_json(queue, {**item, "force": False, "cleanup": {}})
            current = folder / "active-request.json"
            write_json(current, item)
            write_json(state, {"phase": "running", "queued": False, "pid": os.getpid(), "updated": time.time()})
            guard.__exit__(None, None, None)
            guard = None
            log_path = folder / "generation.log"
            with log_path.open("w", encoding="utf-8") as log:
                flags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
                result = subprocess.run(
                    [sys.executable, "-B", "-u", __file__, "--run", str(folder)],
                    cwd=item["root"], stdout=log, stderr=log, creationflags=flags,
                )
            guard = lock(folder / "queue.lock")
            guard.__enter__()
            latest = read_json(queue, {})
            if result.returncode != 0:
                latest["force"] = latest.get("force", False) or item.get("force", False)
                latest["cleanup"] = {**item.get("cleanup", {}), **latest.get("cleanup", {})}
                write_json(queue, latest)
            if latest["revision"] != revision:
                continue
            write_json(state, {"phase": "complete" if result.returncode == 0 else "failed",
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
        generate(Path(item["root"]), Path(item["environment"]), folder, item.get("force", False), item.get("cleanup", {}))
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
            request(folder, Path(sys.argv[2]), Path(sys.argv[3]), "--force" in sys.argv[4:], approved)
        except BaseException:
            with (folder / "generation.log").open("a", encoding="utf-8") as log:
                traceback.print_exc(file=log)
            write_json(folder / "status.json", {"phase": "failed", "queued": False, "updated": time.time()})
            raise
