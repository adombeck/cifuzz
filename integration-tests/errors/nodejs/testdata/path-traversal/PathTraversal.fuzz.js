const fs = require("fs");

test.fuzz("Test path traversal", jazzerBuffer => {
	try {
		fs.openSync(jazzerBuffer.toString(), "r");
	} catch (e) {
		// ignore
	}
});
