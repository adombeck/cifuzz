const { danger, warn, markdown } = require("danger");
const { basename, dirname } = require("path");

const goFileFilter = fileName =>
	fileName.includes(".go") &&
	!fileName.includes("_test.go") &&
	!fileName.includes("testutil") &&
	!fileName.startsWith("integration-tests") &&
	!fileName.startsWith("e2e") &&
	!fileName.startsWith(".golangci.yaml");
const testFileFilter = fileName => fileName.includes("_test.go");

const createdGoFiles = danger.git.created_files.filter(goFileFilter);
const createdTestFiles = danger.git.created_files.filter(testFileFilter);
const modifiedGoFiles = danger.git.modified_files.filter(goFileFilter);
const modifiedTestFiles = danger.git.modified_files.filter(testFileFilter);

// Warnings
checkDescription();
prSize();
missingTestsForCreatedFiles();
missingTestsForModifiedFiles();
largeFiles();

// Encouragement
newTestsForExistingFiles();
removedMoreCodeThanAdded();

function checkDescription() {
	if (!danger.github.pr.body || danger.github.pr.body.length <= 0) {
		warn(`This PR doesn't have a description.
    We recommend following the template to include all necessary information.`);
	} else {
		if (
			danger.github.pr.body?.includes("Motivation/Context") &&
			danger.github.pr.body?.includes("Description") &&
			danger.github.pr.body?.includes("How to use/reproduce")
		) {
			markdown("Thank you for using the PR template ❤️");
		}
	}
}

function prSize() {
	if (danger.github.pr.changed_files > 15) {
		warn(`This PR changes a lot of files (${danger.github.pr.changed_files}).
      It could be useful to break it up into multiple PRs to keep your changes simple and easy to review.`);
	}
}

function missingTestsForCreatedFiles() {
	if (createdGoFiles?.length > 0) {
		const missingTestsForCreatedGoFiles = createdGoFiles.filter(x => {
			// Create the test file names for the go file and check
			// if it can be found in the list of created test files
			const filePath = dirname(x);
			const testFile = basename(x).replace(".go", "_test.go");
			return !createdTestFiles.includes(`${filePath}/${testFile}`);
		});

		// No idea why these extra lines are necessary
		// but without them the bullet points *sometimes* don't work
		if (missingTestsForCreatedGoFiles?.length > 0) {
			warn(`
  The following created files don't have corresponding test files:
  - [ ] ${missingTestsForCreatedGoFiles.join("\n - [ ] ")}

  If you checked the file and there is no need for the test, you can tick the checkbox.`);
		}
	}
}

function missingTestsForModifiedFiles() {
	if (modifiedGoFiles?.length > 0) {
		const missingTestsForModifiedGoFiles = modifiedGoFiles.filter(x => {
			// Create the test file names for the go file and check
			// if it can be found in the list of modified or created test files
			const filePath = dirname(x);
			const testFile = basename(x).replace(".go", "_test.go");
			return (
				!modifiedTestFiles.includes(`${filePath}/${testFile}`) && !createdTestFiles.includes(`${filePath}/${testFile}`)
			);
		});

		// No idea why these extra lines are necessary
		// but without them the bullet points *sometimes* don't work
		if (missingTestsForModifiedGoFiles?.length > 0) {
			warn(`
  The following files have been modified but their tests have not changed:
  - [ ] ${missingTestsForModifiedGoFiles.join("\n - [ ] ")}

  If you checked the file and there is no need for the test, you can tick the checkbox.`);
		}
	}
}

async function largeFiles() {
	const largeFiles = [];
	for (const file of danger.git.created_files) {
		const lines = await danger.git.linesOfCode(file);
		if (lines > 500) {
			largeFiles.push(file);
		}
	}

	if (largeFiles?.length > 0) {
		warn(`
  The following newly created files are very large:
  - [ ] ${largeFiles.join("\n - [ ] ")}

  Please check if this is an intended change.`);
	}
}

function newTestsForExistingFiles() {
	if (createdTestFiles?.length > 0) {
		const expandedTestCoverage = createdTestFiles.filter(x => {
			// Create the go file names for the test file and check
			// if it can be found in the list of created go files
			const filePath = dirname(x);
			const file = basename(x).replace("_test.go", ".go");
			return !createdGoFiles.includes(`${filePath}/${file}`);
		});

		if (expandedTestCoverage?.length > 0) {
			markdown(`You're a rockstar for creating tests for files without any ⭐`);
		}
	}
}

function removedMoreCodeThanAdded() {
	if (danger.github.pr.deletions > danger.github.pr.additions) {
		markdown(`You removed more lines of code than you added, nice cleanup 🧹`);
	}
}
