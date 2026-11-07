from pathlib import Path
import platform
import subprocess
import unittest


class NativeTests(unittest.TestCase):
    def test_accepts_host_and_rejects_emulated_target(self):
        arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
        if arch is None:
            self.skipTest("Native workflow tests target amd64 and arm64 hosts")
        other = "arm64" if arch == "amd64" else "amd64"
        script = Path(__file__).parents[1] / "check-native.sh"
        good = subprocess.run(["bash", str(script), arch], capture_output=True, text=True)
        bad = subprocess.run(["bash", str(script), other], capture_output=True, text=True)
        self.assertEqual(good.returncode, 0, good.stderr)
        self.assertNotEqual(bad.returncode, 0)
        self.assertIn("emulation is not supported", bad.stderr)
