package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// blocks are the README's fenced blocks of language lang, in order.
func blocks(t *testing.T, readme, lang string) []string {
	t.Helper()
	re := regexp.MustCompile("(?s)```" + lang + "\n(.*?)```")
	var out []string
	for _, m := range re.FindAllStringSubmatch(readme, -1) {
		out = append(out, m[1])
	}
	return out
}

func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The README shows a file, a command and what it draws: all three must be
// what this command does with that file.
func TestTheReadmeYAMLExampleIsWhatItDraws(t *testing.T) {
	md := readme(t)
	file, err := os.ReadFile("../../examples/release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if y := blocks(t, md, "yaml"); len(y) == 0 || y[0] != string(file) {
		t.Errorf("the README's YAML is not examples/release.yaml")
	}
	if !strings.Contains(md, "cligram -width 80 examples/release.yaml") {
		t.Error("the README does not show the command")
	}
	var out, errs bytes.Buffer
	if code := run([]string{"-width", "80", "../../examples/release.yaml"}, nil, &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	if text := blocks(t, md, "text"); len(text) == 0 || text[0] != out.String() {
		t.Errorf("the README's drawing is not what the command draws:\n%s", out.String())
	}
}

// The README's Go example and its drawing are Example in example_test.go,
// which go test runs and checks.
func TestTheReadmeGoExampleIsTheTestedExample(t *testing.T) {
	md := readme(t)
	src, err := os.ReadFile("../../example_test.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)func Example\(\) \{\n(.*?)\n\t// Output:\n(.*?)\n\}`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("no Example in example_test.go")
	}
	body := strings.ReplaceAll(strings.TrimSpace(m[1]), "\n\t", "\n")
	var output []string
	for _, line := range strings.Split(m[2], "\n") {
		output = append(output, strings.TrimPrefix(strings.TrimPrefix(line, "\t//"), " "))
	}
	if g := blocks(t, md, "go"); len(g) == 0 || strings.TrimSpace(g[0]) != body {
		t.Errorf("the README's Go is not Example's body:\n%s", body)
	}
	if text := blocks(t, md, "text"); len(text) < 2 || strings.TrimRight(text[1], "\n") != strings.Join(output, "\n") {
		t.Errorf("the README's Go drawing is not Example's output:\n%s", strings.Join(output, "\n"))
	}
}

func TestTheCommandSaysWhatIsWrong(t *testing.T) {
	for _, tc := range []struct {
		args []string
		in   string
		code int
		says string
	}{
		{nil, "", 2, "usage: cligram"},
		{[]string{"missing.yaml"}, "", 1, "no such file"},
		{[]string{"-"}, "nodes: {a: A}\nedges: [a -> ghost]\n", 1, `"ghost" is not a node`},
		{[]string{"-color", "always", "-"}, "nodes: {a: A}\n", 0, ""},
		{[]string{"-color", "purple", "-"}, "nodes: {a: A}\n", 2, "-color is auto, always or never"},
	} {
		var out, errs bytes.Buffer
		code := run(tc.args, strings.NewReader(tc.in), &out, &errs)
		if code != tc.code || !strings.Contains(errs.String(), tc.says) {
			t.Errorf("%v: exit %d, %q", tc.args, code, errs.String())
		}
	}
}

func TestColorIsForTerminalsOrWhenAsked(t *testing.T) {
	in := "nodes: {a: A, b: B}\nedges: [a -> b]\n"
	var plain, colored, errs bytes.Buffer
	run([]string{"-"}, strings.NewReader(in), &plain, &errs)
	run([]string{"-color", "always", "-"}, strings.NewReader(in), &colored, &errs)
	if strings.Contains(plain.String(), "\x1b[") || !strings.Contains(colored.String(), "\x1b[") {
		t.Errorf("plain %q\ncolored %q", plain.String(), colored.String())
	}
}

func TestASCIIDrawsWithASCIIOnly(t *testing.T) {
	var out, errs bytes.Buffer
	run([]string{"-ascii", "-"}, strings.NewReader("nodes: {a: A, b: B}\nedges: [a -> b]\n"), &out, &errs)
	for _, r := range out.String() {
		if r > 127 {
			t.Fatalf("%q in %q", r, out.String())
		}
	}
}
