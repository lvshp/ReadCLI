#!/usr/bin/env python3
"""Resolve a release version and notes from the checked-out Actions commit."""

import os
import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
STABLE_VERSION = re.compile(r"v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)")


def git(root, *args):
    return subprocess.check_output(["git", *args], cwd=root, text=True).strip()


def changelog_notes(root, version):
    path = root / "CHANGELOG.md"
    if not path.exists():
        return ""
    sections = re.split(r"(?m)^## ", path.read_text(encoding="utf-8"))
    for section in sections[1:]:
        heading, _, body = section.partition("\n")
        if re.match(rf"\[?{re.escape(version)}\]?(?:\s|$)", heading):
            return body.strip()
    return ""


def prepare(root, env):
    event, ref = env["GITHUB_EVENT_NAME"], env["GITHUB_REF"]
    sha = git(root, "rev-parse", "HEAD")
    if sha != env["GITHUB_SHA"]:
        raise ValueError("Checkout does not match the triggering commit")

    if event in ("push", "workflow_dispatch") and ref == "refs/heads/dev":
        base = (root / ".github/prerelease-version").read_text(encoding="utf-8").strip()
        if not STABLE_VERSION.fullmatch(base):
            raise ValueError(".github/prerelease-version must contain vMAJOR.MINOR.PATCH")
        run, attempt = env["GITHUB_RUN_NUMBER"], env["GITHUB_RUN_ATTEMPT"]
        if not all(re.fullmatch(r"[1-9]\d*", value) for value in (run, attempt)):
            raise ValueError("Invalid Actions run number or attempt")
        version = f"{base}-dev.{run}.{attempt}"
        prerelease, latest = "true", "false"
        notes = (root / ".github/release-notes/dev.md").read_text(encoding="utf-8").strip()
        if not notes:
            raise ValueError("Development release notes must not be empty")
        run_url = f"{env['GITHUB_SERVER_URL']}/{env['GITHUB_REPOSITORY']}/actions/runs/{env['GITHUB_RUN_ID']}"
        notes += f"\n\n---\n构建版本：`{version}` · 分支：`dev` · 提交：`{sha}`\n\n[构建记录]({run_url})\n"
    elif event == "push" and ref.startswith("refs/tags/"):
        version = ref.removeprefix("refs/tags/")
        if not STABLE_VERSION.fullmatch(version):
            raise ValueError("Stable releases require a vMAJOR.MINOR.PATCH tag")
        if git(root, "rev-parse", f"{ref}^{{commit}}") != sha:
            raise ValueError("Release tag does not match the triggering commit")
        subprocess.run(["git", "merge-base", "--is-ancestor", sha, "origin/main"], cwd=root, check=True)
        prerelease, latest = "false", "legacy"
        notes = ""
        if git(root, "cat-file", "-t", ref) == "tag":
            notes = git(root, "for-each-ref", ref, "--format=%(contents)")
        notes = notes or changelog_notes(root, version)
        if not notes:
            raise ValueError("Add annotated tag notes or a matching CHANGELOG section before releasing")
    else:
        raise ValueError("Use a push to dev, a stable version tag, or run this workflow on dev")

    return {"version": version, "sha": sha, "prerelease": prerelease, "make_latest": latest}, notes


if __name__ == "__main__":
    outputs, body = prepare(ROOT, os.environ)
    (ROOT / "release_notes.md").write_text(body + "\n", encoding="utf-8")
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
        for name, value in outputs.items():
            stream.write(f"{name}={value}\n")
    print(f"Prepared {outputs['version']} at {outputs['sha']} (prerelease={outputs['prerelease']})")
