import importlib
import os
import tempfile
import unittest
from unittest import mock


class PythonPackageTests(unittest.TestCase):
    def setUp(self):
        self.env_patcher = mock.patch.dict(os.environ, {}, clear=False)
        self.env_patcher.start()
        os.environ.pop("SLOP_MCP_VERSION", None)
        self.home_dir = tempfile.TemporaryDirectory()
        os.environ["HOME"] = self.home_dir.name

    def tearDown(self):
        self.home_dir.cleanup()
        self.env_patcher.stop()

    def reload_package(self):
        import slop_mcp

        return importlib.reload(slop_mcp)

    @mock.patch("platform.machine", return_value="x86_64")
    @mock.patch("platform.system", return_value="Linux")
    def test_download_url_uses_env_version_override(self, _system, _machine):
        os.environ["SLOP_MCP_VERSION"] = "1.2.3"
        slop_mcp = self.reload_package()

        self.assertEqual(
            "https://github.com/standardbeagle/slop-mcp/releases/download/v1.2.3/slop-mcp-linux-amd64",
            slop_mcp.get_download_url(),
        )

    @mock.patch("platform.machine", return_value="x86_64")
    @mock.patch("platform.system", return_value="Linux")
    def test_download_url_rejects_placeholder_version(self, _system, _machine):
        slop_mcp = self.reload_package()

        with mock.patch.object(slop_mcp, "__version__", "0.0.0"):
            with self.assertRaisesRegex(RuntimeError, "Cannot determine released"):
                slop_mcp.get_download_url()

    @mock.patch("platform.machine", return_value="arm64")
    @mock.patch("platform.system", return_value="Darwin")
    def test_binary_path_uses_version_override(self, _system, _machine):
        os.environ["SLOP_MCP_VERSION"] = "2.0.0"
        slop_mcp = self.reload_package()

        path = slop_mcp.get_binary_path()

        self.assertEqual("slop-mcp-2.0.0-darwin-arm64", path.name)


if __name__ == "__main__":
    unittest.main()
