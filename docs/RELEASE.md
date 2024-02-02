## How to create a new cifuzz release

---

> **IMPORTANT:** Before publishing a new release, please make sure that there
> are no [open tickets with the `RELEASE_BLOCKER` tag in
> Jira](https://code-intelligence.atlassian.net/issues/?jql=labels%20%3D%20%22RELEASE_BLOCKER%22).

---

The process for creating a new cifuzz release is semi-automated.

### Step 1: Create a version tag

First of all, make sure you are on the latest version of the `main` branch:

    git checkout main
    git pull

(Optional) Check what tags already exist:

    git tag

Create a new tag with a new version number, e.g.:

    git tag v0.6.0 -m "Version 0.6.0"

Please make sure to prefix version tags with a `v` as shown above.

### Step 2: Push the version tag to trigger the release pipeline

Push the new tag to origin:

    git push origin main --tags

Pushing a version tag will trigger an automatic [release
pipeline](https://github.com/adombeck/cifuzz/actions/workflows/pipeline_release.yml)
on GitHub.

After the pipeline has passed, a draft release is automatically created,
including binary artifacts for all supported platforms.

You can find the draft release on the [cifuzz releases page on
GitHub](https://github.com/adombeck/cifuzz/releases).

### Step 3: Fill in a short summary

The release notes will be automatically filled by the GitHub action.

However, you can add a few bullets with the most important changes as a
**Summary**.

### Step 4: Publish the release

After you have checked the draft release and maybe added a short summary at the
top, go ahead and publish the release.

**Please make sure to:**

- check if the version tag and the release name have the same version notation
- the published release is _set as the latest release_.
