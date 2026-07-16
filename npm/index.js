const os = require("os");
const path = require("path");
const fs = require("fs");

const BINARY_NAME = "slop-mcp";

function getPlatformInfo() {
  const platform = os.platform();
  const arch = os.arch();

  let goos;
  if (platform === "darwin") {
    goos = "darwin";
  } else if (platform === "linux") {
    goos = "linux";
  } else if (platform === "win32") {
    goos = "windows";
  } else {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  let goarch;
  if (arch === "x64") {
    goarch = "amd64";
  } else if (arch === "arm64") {
    goarch = "arm64";
  } else {
    throw new Error(`Unsupported architecture: ${arch}`);
  }

  return { goos, goarch };
}

function getBinaryPath() {
  const { goos, goarch } = getPlatformInfo();
  const ext = goos === "windows" ? ".exe" : "";
  const binaryName = `${BINARY_NAME}-${goos}-${goarch}${ext}`;

  // Check for bundled binary first
  const bundledPath = path.join(__dirname, "binaries", binaryName);
  if (fs.existsSync(bundledPath)) {
    // Ensure the binary is executable before we hand it to spawn. The
    // postinstall chmod (install.js) is skipped whenever the consumer sets
    // npm `ignore-scripts=true`, which leaves the binary at 0644 and makes the
    // spawn fail with EACCES. Doing it here (at resolve time, every run) is
    // immune to that. Best-effort: a genuinely unwritable/already-exec binary
    // still spawns, and spawn surfaces a clear error if it truly cannot run.
    if (goos !== "windows") {
      try {
        fs.chmodSync(bundledPath, 0o755);
      } catch (err) {
        // ignore — spawn will report the real problem if there is one
      }
    }
    return bundledPath;
  }

  throw new Error(
    `Binary not found: ${binaryName}. Please reinstall the package.`
  );
}

module.exports = { getBinaryPath, getPlatformInfo };
