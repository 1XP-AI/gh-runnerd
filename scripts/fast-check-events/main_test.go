package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitSelectorPreservesGoMatcherBoundaries(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		want    [][]string
	}{
		{pattern: "A/B", want: [][]string{{"A", "B"}}},
		{pattern: "[/]/[(]", want: [][]string{{"[/]", "[(]"}}},
		{pattern: "([)/][(])", want: [][]string{{"([)/][(])"}}},
		{pattern: "A/B|C/D", want: [][]string{{"A", "B"}, {"C", "D"}}},
		{pattern: `A\/B`, want: [][]string{{`A\/B`}}},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			if got := splitSelector(tc.pattern); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitSelector(%q) = %#v, want %#v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestSelectorMatchesCompleteNames(t *testing.T) {
	for _, tc := range []struct {
		selector string
		name     string
		want     bool
	}{
		{selector: "^TestParent$/^Actual$", name: "TestParent", want: false},
		{selector: "^TestParent$/^Actual$", name: "TestParent/Actual", want: true},
		{selector: "^TestParent$/^Actual$", name: "TestParent/Actual/Grandchild", want: true},
		{selector: "^TestParent$/^Actual$", name: "TestParent/Missing", want: false},
		{selector: "^TestParent$/^Actual[/]?$", name: "TestParent/Actual", want: true},
		{selector: "^TestParent$/^Actual(/)?$", name: "TestParent/Actual", want: true},
		{selector: "^TestParent$/^(Actual|Other)$", name: "TestParent/Other", want: true},
		{selector: "^TestParent$/^Actual$|^TestOther$/^Child$", name: "TestOther/Child", want: true},
		{selector: "^TestParent$/^Actual$|^TestOther$/^Child$", name: "TestParent/Missing", want: false},
	} {
		t.Run(tc.selector+"/"+tc.name, func(t *testing.T) {
			selector, err := compileSelector(tc.selector)
			if err != nil {
				t.Fatal(err)
			}
			if got := selector.matches(tc.name); got != tc.want {
				t.Fatalf("selector %q name %q matched %t, want %t", tc.selector, tc.name, got, tc.want)
			}
		})
	}
}

func TestHasCompleteMatchIgnoresAncestors(t *testing.T) {
	selector, err := compileSelector("^TestParent$/^Missing$")
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("\x16=== RUN   TestParent\n\x16--- PASS: TestParent (0.00s)\n")
	found, err := hasCompleteMatch(input, selector)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("ancestor run event counted as a complete selector match")
	}
}

func TestHasCompleteMatchFindsChild(t *testing.T) {
	selector, err := compileSelector("^TestParent$/^Actual$")
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("\x16=== RUN   TestParent\n\x16=== RUN   TestParent/Actual\n\x16--- PASS: TestParent/Actual (0.00s)\n")
	found, err := hasCompleteMatch(input, selector)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("complete selector run event was not recognized")
	}
}

func TestHasCompleteMatchHandlesFramingBoundaries(t *testing.T) {
	selector, err := compileSelector("^TestSelected$")
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("ordinary output\x16=== RUN   TestSelected\x16--- PASS: TestSelected (0.00s)\n")
	found, err := hasCompleteMatch(input, selector)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("framed test events after ordinary output were not recognized")
	}
}

func TestHasCompleteMatchRequiresFramedPass(t *testing.T) {
	selector, err := compileSelector("^TestSelected$")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "synthetic run has no terminal evidence",
			input: "\x16=== RUN   TestSelected\n",
			want:  false,
		},
		{
			name:  "unframed run and pass are ignored",
			input: "=== RUN   TestSelected\n--- PASS: TestSelected (0.00s)\n",
			want:  false,
		},
		{
			name:  "unmatched framed run and pass are ignored",
			input: "\x16=== RUN   TestOther\n\x16--- PASS: TestOther (0.00s)\n",
			want:  false,
		},
		{
			name:  "skip is not a passing terminal",
			input: "\x16=== RUN   TestSelected\n\x16--- SKIP: TestSelected (0.00s)\n",
			want:  false,
		},
		{
			name:  "first fail terminal blocks a later pass in the same run",
			input: "\x16=== RUN   TestSelected\n\x16--- FAIL: TestSelected (0.00s)\n\x16--- PASS: TestSelected (0.00s)\n",
			want:  false,
		},
		{
			name:  "first skip terminal blocks a later pass in the same run",
			input: "\x16=== RUN   TestSelected\n\x16--- SKIP: TestSelected (0.00s)\n\x16--- PASS: TestSelected (0.00s)\n",
			want:  false,
		},
		{
			name:  "pass without run is ignored",
			input: "\x16--- PASS: TestSelected (0.00s)\n",
			want:  false,
		},
		{
			name:  "framed run and pass qualify",
			input: "\x16=== RUN   TestSelected\n\x16--- PASS: TestSelected (0.00s)\n",
			want:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			found, err := hasCompleteMatch(strings.NewReader(tc.input), selector)
			if err != nil {
				t.Fatal(err)
			}
			if found != tc.want {
				t.Fatalf("hasCompleteMatch() = %t, want %t", found, tc.want)
			}
		})
	}
}

func TestHasCompleteMatchAcceptsSameNameFromLaterPackage(t *testing.T) {
	selector, err := compileSelector("^TestSelected$")
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("\x16=== RUN   TestSelected\n\x16--- SKIP: TestSelected (0.00s)\n\x16=== RUN   TestSelected\n\x16--- PASS: TestSelected (0.00s)\n")
	found, err := hasCompleteMatch(input, selector)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("same-name passing test in a later package was not recognized")
	}
}
