# Steps to prepare a release

1. On `hotfix` branch:
   * Update ReleaseNotes.md
   * Update version in Makefile
   * Commit, push, and pull request
   * Merge in master

2. On Linux host:
   * Pull master
   * Ensure that GitHub Token environment variable `GITHUB_TOKEN` is set
   * Create release tag: `git tag v1.1.0 -m release`
   * Execute `goreleaser release`
