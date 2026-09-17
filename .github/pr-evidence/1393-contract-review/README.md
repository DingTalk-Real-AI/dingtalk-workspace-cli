# PR #1393 evidence

Tested commit: `88cf15bd6f21e8cc8ddc8dcba44d432f15298172`

Local `DWS_PACKAGE_VERSION=0.0.0-test` Go tests. Not `/eval` and not GitHub Actions page screenshots.

- Agent (contract): `go test ./internal/app -run '^TestAssembledSchemaContractReviewToolsAreRetired$'`
- Agent (shared): `go test ./test/unit -run '^TestSkillDocsDoNotRecommendRetiredCommands$'`
- Command CI (contract): `go test ./internal/helpers -run 'TestCrossPlatformCoverageContract'`
