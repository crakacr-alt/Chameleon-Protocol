# Versioning

Chameleon uses semantic versioning:

- `0.x` — active protocol/product development;
- `1.x` — user-facing stable line;
- `2.x` — mesh/session architecture generation.

Every stable release must have:

1. a version entry in `CHANGELOG.md`;
2. a matching `VERSION` value;
3. a frozen branch `versions/vX.Y.Z`;
4. green CI on the exact release commit;
5. migration/compatibility notes when config or wire behavior changes.

Development branches use `release/vX.Y.Z`. Stable snapshots are never force-updated.

## Compatibility policy

Patch releases fix bugs/security issues without intentionally changing the wire format.

Minor releases may add carriers, client modes and optional protocol features. New
features must negotiate or fail explicitly; silent incompatible parsing is not allowed.

Major releases may change architecture and wire/session semantics.

## Release quality gate

A stable release is complete only after all applicable checks pass:

- gofmt;
- go vet;
- go test -race;
- go build;
- deployment shell syntax;
- protocol unit tests;
- end-to-end tunnel/proxy tests;
- upgrade-path tests when installer/config changes;
- documentation and changelog review.

For 1.0 and later, platform-specific client smoke tests are also required.
