import copy
import importlib.util
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("manifest", Path(__file__).parents[1] / "manifest.py")
manifest = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(manifest)
A, B, C = ("sha256:" + char * 64 for char in "abc")


def index():
    return {"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
            "manifests": [
                {"digest": A, "platform": {"os": "linux", "architecture": "amd64"}},
                {"digest": B, "platform": {"os": "linux", "architecture": "arm64", "variant": "v8"}},
                {"digest": C, "platform": {"os": "unknown", "architecture": "unknown"},
                 "annotations": {"vnd.docker.reference.type": "attestation-manifest",
                                 "vnd.docker.reference.digest": A}},
            ]}


class DigestTests(unittest.TestCase):
    def test_retry_replaces_record_and_retains_two_arches(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest.record("amd64", C, tmp)
            manifest.record("amd64", A, tmp)
            manifest.record("arm64", B, tmp)
            self.assertEqual(manifest.references(tmp, "ghcr.io/example/image"),
                             [f"ghcr.io/example/image@{A}", f"ghcr.io/example/image@{B}"])

    def test_rejects_missing_duplicate_malformed_and_extra_digest(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest.record("amd64", A, tmp)
            with self.assertRaises(ValueError):
                manifest.references(tmp, "image")
            manifest.record("arm64", A, tmp)
            with self.assertRaises(ValueError):
                manifest.references(tmp, "image")
            Path(tmp, "arm64.digest").write_text("bad")
            with self.assertRaises(ValueError):
                manifest.references(tmp, "image")
            manifest.record("arm64", B, tmp)
            Path(tmp, "extra.digest").write_text(C)
            with self.assertRaises(ValueError):
                manifest.references(tmp, "image")

    def test_rejects_invalid_record_before_writing(self):
        with tempfile.TemporaryDirectory() as tmp:
            for arch, value in [("../other", A), ("amd64", "sha256:short"), ("arm64", A + "\n")]:
                with self.subTest(arch=arch, value=value), self.assertRaises(ValueError):
                    manifest.record(arch, value, tmp)
            self.assertEqual(list(Path(tmp).iterdir()), [])


class IndexTests(unittest.TestCase):
    def test_accepts_both_native_platforms_with_attestation(self):
        self.assertEqual(manifest.verify_index(index()), ["linux/amd64", "linux/arm64"])

    def test_rejects_missing_duplicate_unexpected_and_unbound_attestation(self):
        cases = []
        missing = index()
        missing["manifests"].pop(1)
        cases.append(missing)
        duplicate = index()
        duplicate["manifests"].append(copy.deepcopy(duplicate["manifests"][0]))
        cases.append(duplicate)
        wrong = index()
        wrong["manifests"][1]["platform"]["architecture"] = "386"
        cases.append(wrong)
        unbound = index()
        unbound["manifests"][2]["annotations"]["vnd.docker.reference.digest"] = C
        cases.append(unbound)
        anonymous = index()
        anonymous["manifests"][2]["annotations"] = {}
        cases.append(anonymous)
        for value in cases:
            with self.subTest(value=value), self.assertRaises(ValueError):
                manifest.verify_index(value)

    def test_rejects_malformed_index_shape(self):
        for value in [[], {}, {"schemaVersion": 2, "mediaType": index()["mediaType"], "manifests": [None]}]:
            with self.subTest(value=value), self.assertRaises(ValueError):
                manifest.verify_index(value)
