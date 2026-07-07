const assert = require("assert");

const { exitCodeForClose } = require("./bin.js");

assert.strictEqual(exitCodeForClose(0, null), 0);
assert.strictEqual(exitCodeForClose(42, null), 42);

assert.strictEqual(exitCodeForClose(null, "SIGTERM"), 143);

assert.strictEqual(exitCodeForClose(null, "SIGUNKNOWN"), 1);
assert.strictEqual(exitCodeForClose(null, null), 1);
