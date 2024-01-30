test.fuzz("My fuzz test", jazzerBuffer => {
	if (jazzerBuffer.toString() == "Fuzz") {
		throw new Error("Crash!");
	}
});

test.fuzz("My other fuzz test", jazzerBuffer => {
	if (jazzerBuffer.toString() == "Fuzz") {
		throw new Error("Crash!");
	}
});
