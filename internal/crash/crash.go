// Package crash keeps what a fuzz worker says as it crashes.
//
// Go sends a fuzz worker's own output nowhere, so a fatal error (a stack
// overflow, the runtime out of memory) or a panic outside the test's
// goroutine leaves only "fuzzing process hung or terminated unexpectedly:
// exit status 2", and the input it was given, which may not crash alone.
package crash

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

// Dir is where a crashing fuzz worker writes its trace, relative to the
// package being fuzzed: beside its corpus, which CI keeps when fuzzing
// fails. Each worker writes its own file, crash-<pid>.txt.
var Dir = filepath.Join("testdata", "fuzz")

// KeepTrace makes this process, when it is a fuzz worker, write the trace
// of any crash to a file in Dir. A file left empty is a worker that did
// not crash.
func KeepTrace(f *testing.F) {
	if w := flag.Lookup("test.fuzzworker"); w == nil || w.Value.String() != "true" {
		return
	}
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		f.Fatal(err)
	}
	name := filepath.Join(Dir, fmt.Sprintf("crash-%d.txt", os.Getpid()))
	out, err := os.Create(name)
	if err != nil {
		f.Fatal(err)
	}
	if err := debug.SetCrashOutput(out, debug.CrashOptions{}); err != nil {
		f.Fatal(err)
	}
	// The runtime keeps its own copy of the file.
	if err := out.Close(); err != nil {
		f.Fatal(err)
	}
}
