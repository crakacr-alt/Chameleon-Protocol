// Package version contains the human-readable Chameleon release version.
//
// The value is intentionally explicit instead of being derived from git at
// runtime. Release branches update it together with CHANGELOG.md, so binaries
// built from a source archive still report the correct version.
package version

const Current = "1.1.0"
