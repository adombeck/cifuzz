test.fuzz("Test timeout", jazzerBuffer => {
	if (jazzerBuffer.toString() == "Fuzz") {
		while (true) {}
	}
});
