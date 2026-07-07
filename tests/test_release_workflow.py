import re
import unittest
from pathlib import Path


class ReleaseWorkflowTests(unittest.TestCase):
    def test_release_workflow_go_version_matches_module(self):
        root = Path(__file__).resolve().parents[1]
        go_mod = (root / "go.mod").read_text(encoding="utf-8")
        release_yml = (root / ".github" / "workflows" / "release.yml").read_text(
            encoding="utf-8"
        )

        module_match = re.search(r"^go\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)$", go_mod, re.M)
        workflow_match = re.search(r"go-version:\s*['\"]([^'\"]+)['\"]", release_yml)

        self.assertIsNotNone(module_match, "go.mod must declare a Go version")
        self.assertIsNotNone(workflow_match, "release workflow must set go-version")
        self.assertEqual(module_match.group(1), workflow_match.group(1))

    def test_pre_commit_go_hooks_use_oauth_build_tag(self):
        root = Path(__file__).resolve().parents[1]
        pre_commit = (root / ".pre-commit-config.yaml").read_text(encoding="utf-8")

        for hook in ("go test", "go build"):
            pattern = rf"entry:\s*{re.escape(hook)}[^\n]*-tags\s+mcp_go_client_oauth"
            self.assertRegex(pre_commit, pattern)


if __name__ == "__main__":
    unittest.main()
