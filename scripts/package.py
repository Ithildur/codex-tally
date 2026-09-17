#!/usr/bin/env python3
"""Build the six standalone release archives using only Python's standard library."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent
NUMBER = r"(?:0|[1-9][0-9]*)"
IDENTIFIER = rf"(?:{NUMBER}|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
SEMVER = re.compile(
    rf"{NUMBER}\.{NUMBER}\.{NUMBER}"
    rf"(?:-(?P<pre>{IDENTIFIER}(?:\.{IDENTIFIER})*))?"
    r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
)


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group()
    source.add_argument("--version", default="0.0.0-dev")
    source.add_argument("--use-git-tag", action="store_true")
    parser.add_argument("--release", action="store_true", help="require a tagged, clean checkout")
    parser.add_argument("--out", type=Path, default=ROOT / "release", help="empty output directory")
    args = parser.parse_args()
    if args.release and not args.use_git_tag:
        parser.error("--release requires --use-git-tag")
    version = args.version
    if args.use_git_tag:
        tags = git("tag", "--points-at", "HEAD").splitlines()
        if len(tags) != 1:
            parser.error("HEAD must have exactly one git tag")
        version = tags[0]
    match = SEMVER.fullmatch(version)
    if match is None:
        parser.error("version must be strict SemVer without a v prefix or numeric leading zeroes")
    if args.release and git("status", "--porcelain", "--untracked-files=normal"):
        parser.error("release builds require a clean checkout")
    output = args.out.resolve()
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        parser.error("output directory must be empty")

    checksums = []
    for system in ("linux", "darwin", "windows"):
        for arch in ("amd64", "arm64"):
            name = f"codex-tally_{version}_{system}_{arch}"
            print(f"Building {name}", flush=True)
            with tempfile.TemporaryDirectory(prefix="codex-tally-") as temporary:
                binary = Path(temporary) / ("codex-dashboard.exe" if system == "windows" else "codex-dashboard")
                env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="0")
                subprocess.run(
                    ["go", "build", "-trimpath", "-ldflags", f"-s -w -X main.buildVersion={version}", "-o", str(binary), "./cmd/codex-dashboard"],
                    cwd=ROOT, env=env, check=True,
                )
                binary.chmod(0o755)
                # Explicit allowlist: never package .state, auth, or public usage snapshots.
                files = [(binary, binary.name)] + [(ROOT / name, name) for name in (
                    "LICENSE", "README.md", "CONTRIBUTING.md", "SECURITY.md",
                    "docs/configuration.md", "docs/pages.md", "docs/scheduling.md",
                    "docs/sharing.md", "docs/metrics.md", "docs/releasing.md",
                )]
                if system == "windows":
                    archive = output / f"{name}.zip"
                    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
                        for file, member in files:
                            bundle.write(file, member)
                else:
                    archive = output / f"{name}.tar.gz"
                    with tarfile.open(archive, "w:gz") as bundle:
                        for file, member in files:
                            bundle.add(file, arcname=member)
                with archive.open("rb") as file:
                    digest = hashlib.file_digest(file, "sha256").hexdigest()
                checksums.append(f"{digest}  {archive.name}\n")
    (output / "SHA256SUMS").write_text("".join(checksums), encoding="utf-8")
    (output / "metadata.json").write_text(
        json.dumps({"version": version, "prerelease": match["pre"] is not None}) + "\n", encoding="utf-8",
    )


if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
