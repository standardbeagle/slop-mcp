const fs = require("fs");
const path = require("path");
const os = require("os");
const { spawnSync } = require("child_process");
const { getBinaryPath } = require("./index.js");

function makeBinariesExecutable() {
  if (os.platform() === "win32") {
    return;
  }

  const binariesDir = path.join(__dirname, "binaries");
  if (!fs.existsSync(binariesDir)) {
    return;
  }

  const files = fs.readdirSync(binariesDir);
  for (const file of files) {
    const filePath = path.join(binariesDir, file);
    try {
      fs.chmodSync(filePath, 0o755);
    } catch (err) {
      // Ignore permission errors; runtime spawn reports the real failure.
    }
  }
}

function coldStartBinary() {
  const binaryPath = getBinaryPath();
  const result = spawnSync(binaryPath, ["version"], {
    stdio: "ignore",
    shell: false,
  });

  if (result.error) {
    throw new Error(`Cold start failed for ${binaryPath}: ${result.error.message}`);
  }
  if (result.status !== 0) {
    throw new Error(`Cold start failed for ${binaryPath}: exit ${result.status}`);
  }
}

function main() {
  makeBinariesExecutable();
  coldStartBinary();
}

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }
}

module.exports = { coldStartBinary, makeBinariesExecutable };
