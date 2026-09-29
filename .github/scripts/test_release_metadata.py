import subprocess
import tempfile
import unittest
from pathlib import Path

from release_metadata import prepare


class ReleaseMetadataTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Release test")
        self.git("config", "user.email", "release-test@example.invalid")
        self.write(".github/prerelease-version", "v0.3.7\n")
        self.write(".github/release-notes/dev.md", "## 新增功能\n\n- 在线搜书\n")
        self.write("CHANGELOG.md", "## [v0.3.7] - 2026-09-29\n\n### 新增\n\n- 新版本功能\n\n## [v0.3.6]\n\n旧功能\n")
        self.git("add", ".")
        self.git("commit", "-qm", "not the release notes")
        self.sha = self.git("rev-parse", "HEAD")
        self.git("update-ref", "refs/remotes/origin/main", self.sha)
        self.env = {
            "GITHUB_EVENT_NAME": "push", "GITHUB_REF": "refs/heads/dev",
            "GITHUB_SHA": self.sha, "GITHUB_RUN_NUMBER": "42", "GITHUB_RUN_ATTEMPT": "1",
            "GITHUB_RUN_ID": "1234", "GITHUB_SERVER_URL": "https://github.com",
            "GITHUB_REPOSITORY": "lvshp/ReadCLI",
        }

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True, stderr=subprocess.DEVNULL).strip()

    def write(self, path, text):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")

    def test_dev_is_prerelease_and_contains_feature_notes(self):
        outputs, notes = prepare(self.root, self.env)
        self.assertEqual(outputs, {"version": "v0.3.7-dev.42.1", "sha": self.sha, "prerelease": "true", "make_latest": "false"})
        self.assertIn("## 新增功能", notes)
        self.assertIn(self.sha, notes)
        self.assertIn("/actions/runs/1234", notes)

    def test_retry_uses_a_new_version(self):
        first, _ = prepare(self.root, self.env)
        self.env["GITHUB_RUN_ATTEMPT"] = "2"
        retry, _ = prepare(self.root, self.env)
        self.assertNotEqual(first["version"], retry["version"])

    def test_manual_run_requires_dev(self):
        self.env["GITHUB_EVENT_NAME"] = "workflow_dispatch"
        self.assertEqual(prepare(self.root, self.env)[0]["prerelease"], "true")
        self.env["GITHUB_REF"] = "refs/heads/main"
        with self.assertRaises(ValueError):
            prepare(self.root, self.env)

    def test_rejects_checkout_mismatch_and_unsafe_versions(self):
        self.env["GITHUB_SHA"] = "0" * 40
        with self.assertRaises(ValueError):
            prepare(self.root, self.env)
        self.env["GITHUB_SHA"] = self.sha
        self.write(".github/prerelease-version", "v0.3.7\nprerelease=false")
        with self.assertRaises(ValueError):
            prepare(self.root, self.env)

    def test_rejects_empty_dev_notes(self):
        self.write(".github/release-notes/dev.md", "\n")
        with self.assertRaises(ValueError):
            prepare(self.root, self.env)

    def test_stable_preserves_annotated_tag_body(self):
        body = "新增对齐切换。\n\n关联 Issue：#2。"
        self.git("tag", "-a", "v0.3.7", "-m", body)
        self.env["GITHUB_REF"] = "refs/tags/v0.3.7"
        outputs, notes = prepare(self.root, self.env)
        self.assertEqual(outputs["prerelease"], "false")
        self.assertEqual(outputs["make_latest"], "legacy")
        self.assertEqual(notes, body)

    def test_lightweight_tag_uses_its_changelog_section(self):
        self.git("tag", "v0.3.7")
        self.env["GITHUB_REF"] = "refs/tags/v0.3.7"
        _, notes = prepare(self.root, self.env)
        self.assertIn("新版本功能", notes)
        self.assertNotIn("旧功能", notes)
        self.assertNotIn("not the release notes", notes)

    def test_rejects_stable_tag_not_merged_to_main(self):
        self.write("feature.txt", "dev only")
        self.git("add", ".")
        self.git("commit", "-qm", "dev feature")
        self.git("tag", "-a", "v0.3.7", "-m", "Unmerged")
        self.env.update(GITHUB_SHA=self.git("rev-parse", "HEAD"), GITHUB_REF="refs/tags/v0.3.7")
        with self.assertRaises(subprocess.CalledProcessError):
            prepare(self.root, self.env)

    def test_never_promotes_prerelease_tags_to_stable(self):
        for tag in ("v0.3.7-dev.42.1", "v0.3.7-beta.1", "v0.3", "v0.3.7-invalid"):
            with self.subTest(tag=tag):
                self.env["GITHUB_REF"] = f"refs/tags/{tag}"
                with self.assertRaises(ValueError):
                    prepare(self.root, self.env)


if __name__ == "__main__":
    unittest.main()
