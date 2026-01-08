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
    return bundledPath;
  }

  throw new Error(
    `Binary not found: ${binaryName}. Please reinstall the package.`
  );
}

module.exports = { getBinaryPath, getPlatformInfo };
