from pathlib import Path
import unittest

import yaml

ROOT = Path(__file__).parents[3]


class UniqueLoader(yaml.BaseLoader):
    pass


def mapping(loader, node):
    result = {}
    for key, value in loader.construct_pairs(node):
        if key in result:
            raise ValueError(f"duplicate YAML key: {key}")
        result[key] = value
    return result


UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, mapping)


def workflow(name):
    return yaml.load((ROOT / ".github/workflows" / name).read_text(), Loader=UniqueLoader)


def step(job, *, name=None, action=None):
    if name is not None:
        return next(s for s in job["steps"] if s.get("name") == name)
    return next(s for s in job["steps"] if s.get("uses", "").startswith(action + "@"))


class WorkflowTests(unittest.TestCase):
    def setUp(self):
        self.release = workflow("release.yml")
        self.pr = workflow("php-runtime.yml")

    def native_matrix(self, job):
        self.assertEqual(job["strategy"]["matrix"]["include"], [
            {"arch": "amd64", "platform": "linux/amd64", "runner": "ubuntu-24.04"},
            {"arch": "arm64", "platform": "linux/arm64", "runner": "ubuntu-24.04-arm"},
        ])
        self.assertEqual(job["strategy"]["fail-fast"], "false")
        self.assertEqual(job["runs-on"], "${{ (matrix.arch == 'arm64' && vars.ACTIONS_RUNNER_ARM64) || (matrix.arch == 'amd64' && vars.ACTIONS_RUNNER) || matrix.runner }}")
        self.assertIn("check-native.sh", step(job, name="Verify native architecture")["run"])
        build = step(job, action="docker/build-push-action")["with"]
        self.assertEqual(build["platforms"], "${{ matrix.platform }}")
        self.assertIn("scope=pma-gateway-${{ matrix.arch }}", build["cache-from"])
        self.assertIn("scope=pma-gateway-${{ matrix.arch }}", build["cache-to"])
        return build

    def test_release_gates_and_shared_version(self):
        self.assertEqual(self.release["on"], {"pull_request": {"types": ["closed"]}})
        self.assertEqual(self.release["concurrency"], {"group": "release-main", "cancel-in-progress": "false"})
        prepare = self.release["jobs"]["prepare"]
        for gate in ["github.event.pull_request.merged == true",
                     "github.event.pull_request.base.ref == github.event.repository.default_branch",
                     "startsWith(github.event.pull_request.head.ref, 'feature/')"]:
            self.assertIn(gate, prepare["if"])
        self.assertEqual(prepare["outputs"]["merge_sha"], "${{ github.event.pull_request.merge_commit_sha }}")
        for value in ["release_required", "docker_tag", "git_tag", "previous_tag"]:
            self.assertEqual(prepare["outputs"][value], "${{ steps.version.outputs." + value + " }}")
        self.assertIn("tag_exists != 'true'", step(prepare, name="Create and push release tag")["if"])

    def test_native_digest_build_and_retry_safe_artifacts(self):
        job = self.release["jobs"]["build"]
        self.assertEqual(job["needs"], "prepare")
        self.assertEqual(job["if"], "needs.prepare.outputs.release_required == 'true'")
        self.assertEqual(job["cache-mode"], "write")
        self.assertNotIn("setup-qemu", (ROOT / ".github/workflows/release.yml").read_text())
        build = self.native_matrix(job)
        for flag in ["push-by-digest=true", "name-canonical=true", "push=true"]:
            self.assertIn(flag, build["outputs"])
        self.assertNotIn("tags", build)
        self.assertIn("BUILD_VERSION=${{ needs.prepare.outputs.docker_tag }}", build["build-args"])
        self.assertIn("BUILD_COMMIT=${{ needs.prepare.outputs.merge_sha }}", build["build-args"])
        self.assertEqual(build["provenance"], "mode=max")
        self.assertEqual(build["sbom"], "true")
        upload = step(job, action="actions/upload-artifact")["with"]
        self.assertEqual(upload["overwrite"], "true")
        self.assertEqual(upload["name"], "release-digest-${{ matrix.arch }}")
        self.assertEqual(upload["if-no-files-found"], "error")

    def test_no_release_or_failed_platform_cannot_publish(self):
        publish = self.release["jobs"]["publish"]
        self.assertEqual(publish["needs"], ["prepare", "build"])
        self.assertEqual(publish["if"], "needs.prepare.outputs.release_required == 'true'")
        # GitHub's default success() plus both dependencies gates all publication.
        self.assertNotIn("always()", publish["if"])
        self.assertNotIn("continue-on-error", publish)
        checkout = step(publish, action="actions/checkout")["with"]
        self.assertEqual(checkout, {"fetch-depth": "0", "ref": "${{ needs.prepare.outputs.merge_sha }}"})
        download = step(publish, action="actions/download-artifact")["with"]
        self.assertEqual(download, {"pattern": "release-digest-*", "path": "/tmp/digests", "merge-multiple": "true"})
        names = [s.get("name") for s in publish["steps"]]
        self.assertLess(names.index("Fetch release tags"), names.index("Generate release notes"))
        self.assertLess(names.index("Validate and publish multi-arch manifest"), names.index("Push Helm chart to GHCR"))
        self.assertLess(names.index("Push Helm chart to GHCR"), names.index("Create or update GitHub Release"))
        run = step(publish, name="Validate and publish multi-arch manifest")["run"]
        self.assertLess(run.index("manifest.py refs"), run.index("imagetools create --dry-run"))
        self.assertLess(run.index("manifest.py verify /tmp/index.json"), run.index('imagetools create "${tags[@]}"'))
        self.assertIn("manifest.py verify /tmp/published-index.json", run)
        tags = step(publish, action="docker/metadata-action")["with"]["tags"]
        for value in ["docker_tag", "git_tag"]:
            self.assertIn("needs.prepare.outputs." + value, tags)
        self.assertIn("type=raw,value=latest", tags)

    def test_pr_builds_both_natively_without_publishing(self):
        self.assertEqual(self.pr["on"], {"pull_request": {"types": ["opened", "synchronize", "reopened"]}})
        self.assertEqual(self.pr["permissions"], {"contents": "read"})
        job = self.pr["jobs"]["verify"]
        build = self.native_matrix(job)
        self.assertEqual(build["load"], "true")
        self.assertEqual(build["push"], "false")
        self.assertIn("check-php-runtime.sh", step(job, name="Verify native PHP and FPM configuration")["run"])
        self.assertFalse(any("docker/login-action" in s.get("uses", "") for s in job["steps"]))
