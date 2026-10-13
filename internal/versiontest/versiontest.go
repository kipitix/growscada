// Package versiontest builds schema versions around a given one for tests of
// how the tools react to a server older or newer than themselves.
package versiontest

import "github.com/kipitix/growscada/contract"

// Older returns a valid version older than v: the previous MINOR, or a late
// MINOR of the previous MAJOR when v is the first of its MAJOR (e.g. 1.0 →
// 0.1000). v must be newer than 0.0.
func Older(v contract.SchemaVersion) contract.SchemaVersion {
	if v.Minor > 0 {
		return contract.SchemaVersion{Major: v.Major, Minor: v.Minor - 1}
	}
	return contract.SchemaVersion{Major: v.Major - 1, Minor: 1000}
}

// Newer returns the next MINOR after v.
func Newer(v contract.SchemaVersion) contract.SchemaVersion {
	return contract.SchemaVersion{Major: v.Major, Minor: v.Minor + 1}
}
