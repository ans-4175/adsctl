module.exports = {
  branches: ["main"],
  repositoryUrl: "https://github.com/imrenagicom/adsctl.git",
  tagFormat: "v${version}",
  plugins: [
    ["@semantic-release/commit-analyzer", { preset: "conventionalcommits" }],
    ["@semantic-release/release-notes-generator", { preset: "conventionalcommits" }],
    [
      "@semantic-release/exec",
      {
        verifyConditionsCmd: "make release-check",
        // Build the real version before semantic-release creates its tag.
        // The tag does not exist yet, so only Git validation is skipped.
        prepareCmd:
          "GORELEASER_CURRENT_TAG=${nextRelease.gitTag} goreleaser release --clean --skip=publish,validate && python3 scripts/check-plugin.py --archives dist",
      },
    ],
    [
      "@semantic-release/github",
      {
        assets: ["dist/*.tar.gz", "dist/*.zip", "dist/checksums.txt"],
        successCommentCondition: false,
        failCommentCondition: false,
        releasedLabels: false,
      },
    ],
  ],
};
