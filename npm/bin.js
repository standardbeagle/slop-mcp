#!/usr/bin/env node

const os = require("os");
const { spawn } = require("child_process");
const { getBinaryPath } = require("./index.js");

function exitCodeForClose(code, signal) {
  if (code !== null && code !== undefined) {
    return code;
  }
  if (signal && os.constants.signals[signal]) {
    return 128 + os.constants.signals[signal];
  }
  return 1;
}

function main() {
  const binaryPath = getBinaryPath();
  const args = process.argv.slice(2);

  const child = spawn(binaryPath, args, {
    stdio: "inherit",
    shell: false,
  });

  child.on("error", (err) => {
    console.error(`Failed to start slop-mcp: ${err.message}`);
    process.exit(1);
  });

  child.on("close", (code, signal) => {
    process.exit(exitCodeForClose(code, signal));
  });
}

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }
}

module.exports = { exitCodeForClose };
