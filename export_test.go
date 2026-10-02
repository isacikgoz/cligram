package cligram

import "testing"

// CheckRoutes lets the external tests check the routes of their layouts.
func CheckRoutes(t *testing.T, l *Layout) { t.Helper(); checkRoutes(t, l) }

// Golden compares got with testdata/name, or rewrites it with -update.
func Golden(t *testing.T, name, got string) { t.Helper(); golden(t, name, got) }
