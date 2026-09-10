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
	input := strings.NewReader(`{"Action":"run","Test":"TestParent"}
{"Action":"pass","Test":"TestParent"}
`)
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
	input := strings.NewReader(`{"Action":"run","Test":"TestParent"}
{"Action":"run","Test":"TestParent/Actual"}
`)
	found, err := hasCompleteMatch(input, selector)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("complete selector run event was not recognized")
	}
}
