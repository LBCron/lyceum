.PHONY: release-snapshot

# Local release build of the four v1 platforms into dist/, nothing published.
release-snapshot:
	goreleaser release --snapshot --clean
