#!/usr/bin/env python3
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

repo = Path(__file__).resolve().parent.parent
module = "github.com/skolldire/go-engine/concurrency"
version = "v1.0.0"
results = Path(os.environ.get("CONCURRENCY_RELEASE_RESULTS_DIR") or tempfile.mkdtemp(prefix="concurrency-release-fixture-results-"))
results.mkdir(parents=True, exist_ok=True)
results = results.resolve()

with tempfile.TemporaryDirectory(prefix="concurrency-release-proxy-") as temporary:
    proxy = Path(temporary)
    versions = proxy / module / "@v"
    versions.mkdir(parents=True)
    (versions / "list").write_text(version + "\n")
    (versions / (version + ".mod")).write_bytes((repo / "concurrency/go.mod").read_bytes())
    (versions / (version + ".info")).write_text(json.dumps({"Version": version, "Time": "2026-11-12T00:00:00Z"}))
    archive_path = versions / (version + ".zip")
    entries = {}
    for line in (repo / "docs/modules/concurrency-candidate.sha256").read_text().splitlines():
        _, relative = line.split("  ", 1)
        source = repo / relative
        entries[module + "@" + version + "/" + str(source.relative_to(repo / "concurrency"))] = source.read_bytes()

    def write_archive():
        with zipfile.ZipFile(archive_path, "w", zipfile.ZIP_DEFLATED) as archive:
            for name, data in entries.items():
                archive.writestr(name, data)

    environment = dict(os.environ, GOPROXY=proxy.as_uri(), GOSUMDB="off")

    def run(name, arguments, expected):
        output = results / name
        output.mkdir(exist_ok=True)
        environment["CONCURRENCY_RELEASE_RESULTS_DIR"] = str(output)
        process = subprocess.run([str(repo / "scripts/check-concurrency-release.sh")] + arguments,
                                 env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        (output / "runner.log").write_text(process.stdout)
        if process.returncode != expected:
            raise RuntimeError(f"{name}: expected exit {expected}, got {process.returncode}: {process.stdout}")
        print(f"{name}: expected exit {expected}")

    write_archive()
    run("valid", [version, "--fixture"], 0)
    if "FIXTURE ONLY" not in (results / "valid/runner.log").read_text():
        raise RuntimeError("fixture mislabeled as published evidence")
    run("reject-local-as-published", [version], 2)
    run("reject-invalid-version", ["v0.1.0", "--fixture"], 2)
    entries[module + "@" + version + "/README.md"] += b"\ncorrupted fixture\n"
    write_archive()
    run("reject-corrupt-artifact", [version, "--fixture"], 1)
    if "README.md: FAILED" not in (results / "reject-corrupt-artifact/minimal.log").read_text():
        raise RuntimeError("corrupt artifact failed for an unexpected reason")

print("Release verifier harness passed; E7 against a published tag remains pending")
print(f"results={results}")
