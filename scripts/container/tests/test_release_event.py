"""Exercise release eligibility and metadata forwarding without publishing."""
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch, Mock
import urllib.error

ROOT = Path(__file__).parents[3]
spec = importlib.util.spec_from_file_location("release_event", ROOT / "scripts/release_event.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
SHA = "a" * 40
IDENTITY = {"repository": "owner/repo", "default_branch": "main", "sha": SHA}
ENV = {"GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "owner/repo", "GITHUB_SHA": SHA,
       "GITHUB_REF": "refs/heads/main", "GITHUB_TOKEN": "test-token"}
EVENT = {"repository": {"full_name": "owner/repo", "default_branch": "main"},
         "after": SHA, "ref": "refs/heads/main", "deleted": False}
PR = {"number": 24, "state": "closed", "merged_at": "2026-11-07T16:00:00Z",
      "merge_commit_sha": SHA, "base": {"ref": "main", "repo": {"full_name": "owner/repo"}},
      "head": {"ref": "feature/#24", "sha": "b" * 40, "repo": {"full_name": "fork/repo"}},
      "title": "Release warnings", "body": "Close #24"}


class ReleaseEventTests(unittest.TestCase):
    def test_trusted_push_and_excluded_events(self):
        self.assertEqual(release.trusted_push(EVENT, ENV), IDENTITY)
        for ref in ("refs/heads/feature/#24", "refs/tags/v1.0.0"):
            with self.subTest(ref=ref):
                self.assertIsNone(release.trusted_push(dict(EVENT, ref=ref), dict(ENV, GITHUB_REF=ref)))
        self.assertIsNone(release.trusted_push(EVENT, dict(ENV, GITHUB_EVENT_NAME="pull_request")))
        self.assertIsNone(release.trusted_push(dict(EVENT, deleted=True), ENV))
        renamed = copy.deepcopy(EVENT)
        renamed["repository"]["default_branch"] = "trunk"
        self.assertIsNone(release.trusted_push(renamed, ENV))

    def test_push_identity_mismatch_fails(self):
        for key, value in (("GITHUB_REPOSITORY", "other/repo"), ("GITHUB_SHA", "b" * 40),
                           ("GITHUB_REF", "refs/heads/other")):
            with self.subTest(key=key), self.assertRaises(ValueError):
                release.trusted_push(EVENT, dict(ENV, **{key: value}))
        with self.assertRaises(ValueError):
            release.trusted_push(dict(EVENT, after="not-sha"), ENV)

    def test_fork_merge_squash_and_rebase_use_final_sha_not_head(self):
        # API merge_commit_sha is the merged tip for each merge strategy.
        for head in ("b" * 40, SHA):
            pr = copy.deepcopy(PR)
            pr["head"]["sha"] = head
            with self.subTest(head=head):
                self.assertTrue(release.eligible(pr, IDENTITY))

    def test_ineligible_prs(self):
        cases = []
        for key, value in (("merged_at", None), ("state", "open"), ("merge_commit_sha", "b" * 40)):
            cases.append(dict(PR, **{key: value}))
        for key, value in (("head", {"ref": "fix/24"}),
                           ("base", {"ref": "other", "repo": {"full_name": "owner/repo"}}),
                           ("base", {"ref": "main", "repo": {"full_name": "other/repo"}})):
            cases.append(dict(PR, **{key: value}))
        for pr in cases:
            with self.subTest(pr=pr):
                self.assertFalse(release.eligible(pr, IDENTITY))

    def test_malformed_api_fields_are_errors(self):
        for key in PR:
            pr = copy.deepcopy(PR)
            del pr[key]
            with self.subTest(missing=key), self.assertRaises((KeyError, TypeError, ValueError)):
                release.eligible(pr, IDENTITY)
        for pr in (dict(PR, number=True), dict(PR, body=[]), dict(PR, title="bad\0title")):
            with self.assertRaises(ValueError):
                release.eligible(pr, IDENTITY)

    @patch.object(release.time, "sleep")
    @patch.object(release, "associated_prs")
    def test_bounded_indexing_retry_then_success(self, fetch, sleep):
        fetch.side_effect = [[], [dict(PR, merged_at=None)], [PR]]
        self.assertEqual(release.resolve(IDENTITY, ENV), dict(IDENTITY, pr=PR))
        self.assertEqual(fetch.call_count, 3)
        self.assertEqual(sleep.call_count, 2)
        sleep.assert_called_with(release.RETRY_SECONDS)

    @patch.object(release.time, "sleep")
    @patch.object(release, "associated_prs", return_value=[])
    def test_direct_push_retries_then_skips(self, fetch, sleep):
        self.assertIsNone(release.resolve(IDENTITY, ENV))
        self.assertEqual(fetch.call_count, release.ATTEMPTS)
        self.assertEqual(sleep.call_count, release.ATTEMPTS - 1)

    @patch.object(release.time, "sleep")
    @patch.object(release, "associated_prs", return_value=[PR, dict(PR, number=25)])
    def test_ambiguity_fails_without_retry(self, fetch, sleep):
        with self.assertRaises(ValueError):
            release.resolve(IDENTITY, ENV)
        sleep.assert_not_called()

    @patch.object(release.time, "sleep")
    @patch.object(release, "associated_prs")
    def test_api_failure_not_skip(self, fetch, sleep):
        for code in (401, 403, 500):
            fetch.side_effect = urllib.error.HTTPError("https://api.github.com", code, "error", {}, None)
            with self.subTest(code=code), self.assertRaises(urllib.error.HTTPError):
                release.resolve(IDENTITY, ENV)
        sleep.assert_not_called()

    @patch.object(release.urllib.request, "urlopen")
    def test_api_pagination_and_invalid_shape(self, urlopen):
        def response(body):
            r = Mock()
            r.__enter__ = Mock(return_value=io.StringIO(json.dumps(body)))
            r.__exit__ = Mock(return_value=False)
            return r
        urlopen.side_effect = [response([PR] * 100), response([dict(PR, number=25)])]
        self.assertEqual(len(release.associated_prs(IDENTITY, ENV)), 101)
        request = urlopen.call_args_list[1].args[0]
        self.assertTrue(request.full_url.endswith("per_page=100&page=2"))
        self.assertEqual(request.get_header("Authorization"), "Bearer test-token")
        self.assertEqual(urlopen.call_args.kwargs["timeout"], 30)
        urlopen.side_effect = [response({"message": "unexpected"})]
        with self.assertRaises(ValueError):
            release.associated_prs(IDENTITY, ENV)

    @patch.object(release.subprocess, "run")
    def test_version_forwarding_is_data_and_preserves_failure(self, run):
        text = 'title\n$(touch /tmp/never) `command` "quote"\nEND\nrelease_required=true'
        pr = dict(PR, title=text, body=text + "\n日本語")
        run.return_value.returncode = 7
        env = dict(ENV, GITHUB_OUTPUT="output", PR_NUMBER="stale", MERGE_COMMIT_SHA="old")
        self.assertEqual(release.compute_version(dict(IDENTITY, pr=pr), IDENTITY, env), 7)
        args, kwargs = run.call_args
        self.assertEqual(args[0], [release.sys.executable, str(ROOT / "scripts/release/compute_release.py")])
        self.assertNotIn("shell", kwargs)
        self.assertEqual(kwargs["env"]["PR_TITLE"], text)
        self.assertEqual(kwargs["env"]["PR_BODY"], text + "\n日本語")
        self.assertEqual(kwargs["env"]["PR_NUMBER"], "24")
        self.assertEqual(kwargs["env"]["MERGE_COMMIT_SHA"], SHA)
        self.assertEqual(kwargs["env"]["GITHUB_OUTPUT"], "output")
        self.assertEqual(env["MERGE_COMMIT_SHA"], "old")

    @patch.object(release.subprocess, "run")
    def test_stale_snapshot_cannot_invoke_helper(self, run):
        for key in IDENTITY:
            snapshot = dict(IDENTITY, pr=PR)
            snapshot[key] = "stale"
            with self.subTest(key=key), self.assertRaises(ValueError):
                release.compute_version(snapshot, IDENTITY, ENV)
        with self.assertRaises(ValueError):
            release.compute_version(dict(IDENTITY, pr=PR), None, ENV)
        run.assert_not_called()

    @patch.object(release, "resolve")
    def test_cli_snapshot_and_only_safe_outputs(self, resolve):
        with tempfile.TemporaryDirectory() as directory:
            event, output, snapshot = [Path(directory) / f for f in ("event.json", "output", "pr.json")]
            event.write_text(json.dumps(EVENT))
            env = dict(ENV, GITHUB_EVENT_PATH=str(event), GITHUB_OUTPUT=str(output))
            resolve.return_value = dict(IDENTITY, pr=dict(PR, body="\npr_number=999\n"))
            with patch.dict(os.environ, env, clear=True), patch.object(release.sys, "argv", ["release_event", "resolve", str(snapshot)]):
                self.assertEqual(release.main(), 0)
                self.assertEqual(output.read_text(), "eligible=true\npr_number=24\n")
                self.assertEqual(json.loads(snapshot.read_text())["pr"]["body"], "\npr_number=999\n")
                resolve.return_value = None
                self.assertEqual(release.main(), 0)
                self.assertFalse(snapshot.exists())
                resolve.side_effect = urllib.error.URLError("unavailable")
                self.assertEqual(release.main(), 1)
                self.assertFalse(snapshot.exists())

    @patch.object(release, "resolve")
    @patch.object(release, "compute_version", return_value=9)
    def test_cli_version_and_nondefault_skip(self, compute, resolve):
        with tempfile.TemporaryDirectory() as directory:
            event, output, snapshot = [Path(directory) / f for f in ("event.json", "output", "pr.json")]
            event.write_text(json.dumps(EVENT))
            snapshot.write_text(json.dumps(dict(IDENTITY, pr=PR)))
            env = dict(ENV, GITHUB_EVENT_PATH=str(event), GITHUB_OUTPUT=str(output))
            with patch.dict(os.environ, env, clear=True), patch.object(release.sys, "argv", ["release_event", "version", str(snapshot)]):
                self.assertEqual(release.main(), 9)
                self.assertEqual(compute.call_args.args[:2], (dict(IDENTITY, pr=PR), IDENTITY))
            ref = "refs/tags/v1.0.0"
            event.write_text(json.dumps(dict(EVENT, ref=ref)))
            with patch.dict(os.environ, dict(env, GITHUB_REF=ref), clear=True), patch.object(release.sys, "argv", ["release_event", "resolve", str(snapshot)]):
                self.assertEqual(release.main(), 0)
                self.assertFalse(snapshot.exists())
                self.assertEqual(output.read_text(), "eligible=false\npr_number=\n")
                resolve.assert_not_called()
