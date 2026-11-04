# Release Workshop and SDKcraft

1. Create release/<version> from the latest origin/main. Ask for `<version>` if missing.
2. Add Workshop release notes for `<version>` to `docs/release-notes/`, covering changes on `main` since Workshop's latest signed release tag. Determine SDKcraft's independently released version from `canonical/sdkcraft`; include its changes when it released, or carry its last version forward as unchanged. Follow the template in `docs/doc-style-guide.md` and update `docs/release-notes/index.rst`.
3. Set the version in `internal/VERSION` to `<version>`.
4. Commit the release changes, push release/<version> to origin, and open a PR targeting main, titled Release <version>. Include a summary in the PR description.
