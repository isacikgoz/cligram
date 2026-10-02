package cligram

import (
	"fmt"
	"testing"
)

// CheckRoutes lets the external tests check the routes of their layouts.
func CheckRoutes(t *testing.T, l *Layout) { t.Helper(); checkRoutes(t, l) }

// Golden compares got with testdata/name, or rewrites it with -update.
func Golden(t *testing.T, name, got string) { t.Helper(); golden(t, name, got) }

// Routes describes a layout's routes, for a failing test to show.
func Routes(l *Layout) []string {
	var out []string
	for _, rt := range l.routes {
		out = append(out, fmt.Sprintf("%s -> %s %q path=%v label=(%d,%d) placed=%v",
			rt.edge.From, rt.edge.To, rt.label, rt.path, rt.labelX, rt.labelY, rt.placed))
	}
	return out
}
