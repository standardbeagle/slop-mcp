"""slop-mcp: MCP server for orchestrating multiple MCP servers."""

import os
import platform
import stat
import subprocess
import sys
from pathlib import Path

import httpx

__version__ = "0.0.0"

REPO = "standardbeagle/slop-mcp"
BINARY_NAME = "slop-mcp"


def get_platform_info() -> tuple[str, str]:
    """Get the current platform and architecture."""
    system = platform.system().lower()
    machine = platform.machine().lower()

    if system == "darwin":
        goos = "darwin"
    elif system == "linux":
        goos = "linux"
    elif system == "windows":
        goos = "windows"
    else:
        raise RuntimeError(f"Unsupported platform: {system}")

    if machine in ("x86_64", "amd64"):
        goarch = "amd64"
    elif machine in ("arm64", "aarch64"):
        goarch = "arm64"
    else:
        raise RuntimeError(f"Unsupported architecture: {machine}")

    return goos, goarch


def get_binary_path() -> Path:
    """Get the path where the binary should be stored."""
    cache_dir = Path.home() / ".cache" / "slop-mcp"
    cache_dir.mkdir(parents=True, exist_ok=True)

    goos, goarch = get_platform_info()
    ext = ".exe" if goos == "windows" else ""

    return cache_dir / f"{BINARY_NAME}-{__version__}-{goos}-{goarch}{ext}"


def get_download_url() -> str:
    """Get the download URL for the current platform."""
    goos, goarch = get_platform_info()
    ext = ".exe" if goos == "windows" else ""

    version = __version__
    if not version.startswith("v"):
        version = f"v{version}"

    return f"https://github.com/{REPO}/releases/download/{version}/{BINARY_NAME}-{goos}-{goarch}{ext}"


def download_binary(dest: Path) -> None:
    """Download the binary for the current platform."""
    url = get_download_url()

    with httpx.Client(follow_redirects=True, timeout=60.0) as client:
        response = client.get(url)
        response.raise_for_status()

        dest.write_bytes(response.content)

        # Make executable on Unix
        if platform.system() != "Windows":
            dest.chmod(dest.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)


def ensure_binary() -> Path:
    """Ensure the binary exists, downloading if necessary."""
    binary_path = get_binary_path()

    if not binary_path.exists():
        download_binary(binary_path)

    return binary_path


def main() -> int:
    """Run the slop-mcp binary with the given arguments."""
    try:
        binary_path = ensure_binary()
    except Exception as e:
        print(f"Error downloading slop-mcp binary: {e}", file=sys.stderr)
        return 1

    result = subprocess.run([str(binary_path)] + sys.argv[1:])
    return result.returncode


if __name__ == "__main__":
    sys.exit(main())
