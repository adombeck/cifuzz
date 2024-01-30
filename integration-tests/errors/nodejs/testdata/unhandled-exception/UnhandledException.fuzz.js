test.fuzz("Test unhandled exception", jazzerBuffer => {
	if (jazzerBuffer.toString() == "Fuzz") {
		throw new Error("Crash!");
	}
});
