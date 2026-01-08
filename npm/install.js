const fs = require("fs");
const path = require("path");
const os = require("os");

// Make the binary executable on Unix systems
if (os.platform() !== "win32") {
  const binariesDir = path.join(__dirname, "binaries");

  if (fs.existsSync(binariesDir)) {
    const files = fs.readdirSync(binariesDir);
    for (const file of files) {
      const filePath = path.join(binariesDir, file);
      try {
        fs.chmodSync(filePath, 0o755);
      } catch (err) {
        // Ignore permission errors
      }
    }
  }
}
