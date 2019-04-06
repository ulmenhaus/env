package ui

import (
	"fmt"
	"strings"

	"github.com/ulmenhaus/env/img/jql/types"
)

type EqualFilter struct {
	Field     string
	Col       int
	Formatted string
}

func (ef *EqualFilter) Applies(e []types.Entry) bool {
	return e[ef.Col].Format("") == ef.Formatted
}

func (ef *EqualFilter) Description() string {
	return fmt.Sprintf("%s = \"%s\"", ef.Field, strings.Replace(ef.Formatted, "\"", "\\\"", -1))
}

func (ef *EqualFilter) Example() (int, string) {
	return ef.Col, ef.Formatted
}

func (ef *EqualFilter) PrimarySuggestion() (string, bool) {
	return ef.Formatted, true
}

type InFilter struct {
	Field     string
	Col       int
	Formatted map[string]bool
}

func (f *InFilter) Applies(e []types.Entry) bool {
	return f.Formatted[e[f.Col].Format("")]
}

func (f *InFilter) keys() []string {
	keys := make([]string, len(f.Formatted))
	i := 0
	for key := range f.Formatted {
		keys[i] = key
		i++
	}
	return keys
}

func (f *InFilter) Description() string {
	return fmt.Sprintf("%s in (%s)", f.Field, strings.Join(f.keys(), ", "))
}

func (f *InFilter) Example() (int, string) {
	return f.Col, f.keys()[0]
}

func (f *InFilter) PrimarySuggestion() (string, bool) {
	return "", false
}

// A ContainsFilter applies when the value of an entry at a given column
// is a case insensitive superstring of the provided formatted query
type ContainsFilter struct {
	Field     string
	Col       int
	Formatted string
}

func (cf *ContainsFilter) Applies(e []types.Entry) bool {
	return strings.Contains(strings.ToLower(e[cf.Col].Format("")), strings.ToLower(cf.Formatted))
}

func (cf *ContainsFilter) Description() string {
	return fmt.Sprintf("%s contains \"%s\"", cf.Field, strings.Replace(cf.Formatted, "\"", "\\\"", -1))
}

func (cf *ContainsFilter) Example() (int, string) {
	return cf.Col, cf.Formatted
}

func (cf *ContainsFilter) PrimarySuggestion() (string, bool) {
	return "", false
}
