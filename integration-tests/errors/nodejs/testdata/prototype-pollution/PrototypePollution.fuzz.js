test.fuzz("Test prototype pollution", jazzerBuffer => {
	if (jazzerBuffer.toString() == "Fuzz") {
		const a = {};
		a.__proto__.polluted = true;
	}
});
