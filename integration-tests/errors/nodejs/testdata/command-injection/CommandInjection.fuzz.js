const child_process = require("child_process");

test.fuzz("Test command injection", jazzerBuffer => {
	try {
		child_process.execSync(jazzerBuffer.toString());
	} catch (e) {
		// ignore
	}
});
