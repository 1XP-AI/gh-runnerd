package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

type compiledSelector [][]*regexp.Regexp

func main() {
	var pattern string
	flag.StringVar(&pattern, "selector", "", "Go test selector")
	flag.Parse()
	if pattern == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "fast-check-events requires one non-empty --selector")
		os.Exit(2)
	}

	selector, err := compileSelector(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid Go test selector: %v\n", err)
		os.Exit(2)
	}
	found, err := hasCompleteMatch(os.Stdin, selector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read Go test events: %v\n", err)
		os.Exit(2)
	}
	if !found {
		os.Exit(1)
	}
}

func compileSelector(pattern string) (compiledSelector, error) {
	parts := splitSelector(pattern)
	compiled := make(compiledSelector, len(parts))
	for alternative, sequence := range parts {
		compiled[alternative] = make([]*regexp.Regexp, len(sequence))
		for element, part := range sequence {
			expression, err := regexp.Compile(rewrite(part))
			if err != nil {
				return nil, fmt.Errorf("alternative %d element %d: %w", alternative, element, err)
			}
			compiled[alternative][element] = expression
		}
	}
	return compiled, nil
}

func splitSelector(pattern string) [][]string {
	alternatives := make([][]string, 0, strings.Count(pattern, "|")+1)
	sequence := make([]string, 0, strings.Count(pattern, "/"))
	start := 0
	characterClassDepth := 0
	parenthesisDepth := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '[':
			characterClassDepth++
		case ']':
			characterClassDepth--
			if characterClassDepth < 0 {
				characterClassDepth = 0
			}
		case '(':
			if characterClassDepth == 0 {
				parenthesisDepth++
			}
		case ')':
			if characterClassDepth == 0 {
				parenthesisDepth--
			}
		case '\\':
			i++
		case '/':
			if characterClassDepth == 0 && parenthesisDepth == 0 {
				sequence = append(sequence, pattern[start:i])
				start = i + 1
			}
		case '|':
			if characterClassDepth == 0 && parenthesisDepth == 0 {
				sequence = append(sequence, pattern[start:i])
				alternatives = append(alternatives, sequence)
				sequence = make([]string, 0, strings.Count(pattern[i+1:], "/"))
				start = i + 1
			}
		}
	}
	sequence = append(sequence, pattern[start:])
	alternatives = append(alternatives, sequence)
	return alternatives
}

func (s compiledSelector) matches(name string) bool {
	elements := strings.Split(name, "/")
	for _, alternative := range s {
		matched := true
		for i, expression := range alternative {
			if i >= len(elements) {
				break
			}
			if !expression.MatchString(elements[i]) {
				matched = false
				break
			}
		}
		if matched && len(elements) >= len(alternative) {
			return true
		}
	}
	return false
}

func hasCompleteMatch(input io.Reader, selector compiledSelector) (bool, error) {
	reader := bufio.NewReader(input)
	found := false
	for {
		line, err := reader.ReadString('\n')
		if len(strings.TrimSpace(line)) != 0 {
			var event testEvent
			if json.Unmarshal([]byte(line), &event) == nil && event.Action == "run" && event.Test != "" && selector.matches(event.Test) {
				found = true
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return found, nil
			}
			return false, err
		}
	}
}

func rewrite(s string) string {
	var rewritten strings.Builder
	for _, r := range s {
		switch {
		case isSpace(r):
			rewritten.WriteByte('_')
		case !strconv.IsPrint(r):
			quoted := strconv.QuoteRune(r)
			rewritten.WriteString(quoted[1 : len(quoted)-1])
		default:
			rewritten.WriteRune(r)
		}
	}
	return rewritten.String()
}

func isSpace(r rune) bool {
	if r < 0x2000 {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0, 0x1680:
			return true
		}
	} else {
		if r <= 0x200a {
			return true
		}
		switch r {
		case 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
			return true
		}
	}
	return false
}
