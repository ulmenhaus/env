package api

import (
	"fmt"
	"strings"

	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

type EqualFilter struct {
	Field     string
	Col       int
	Formatted string
	Not       bool
}

func xor(a, b bool) bool {
	return (a && !b) || (!a && b)
}

func (ef *EqualFilter) Applies(e []types.Entry) bool {
	return xor(e[ef.Col].Format("") == ef.Formatted, ef.Not)
}

func (ef *EqualFilter) Description() string {
	op := "="
	if ef.Not {
		op = "!="
	}
	return fmt.Sprintf("%s %s \"%s\"", ef.Field, op, strings.Replace(ef.Formatted, "\"", "\\\"", -1))
}

func (ef *EqualFilter) Example() (int, string) {
	if ef.Not {
		return ef.Col, ""
	}
	return ef.Col, ef.Formatted
}

func (ef *EqualFilter) PrimarySuggestion() (string, bool) {
	if ef.Not {
		return "", false
	}
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

func slice2map(slice []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range slice {
		m[s] = true
	}
	return m
}

// A ContainsFilter applies when the value of an entry at a given column
// is a case insensitive superstring of the provided formatted query
type ContainsFilter struct {
	Field     string
	Col       int // if set to a negative value will match any field
	Formatted string
	Exact     bool // exact requires a case sensitive match and will use table formatting to support list entries
}

func (cf *ContainsFilter) Applies(e []types.Entry) bool {
	// NOTE exact match + col < 0 not implemented and will cause a panic
	if cf.Exact {
		// HACK to make a ContainsFilter work for ForeignLists format to the full list.
		// NOTE this behavior is only partially correct as it  relies
		// on keys not having newlines
		return strings.Contains(e[cf.Col].Format(types.ListFormat), "\n"+cf.Formatted+"\n")

	}
	if cf.Col < 0 {
		for i := 0; i < len(e); i++ {
			if strings.Contains(strings.ToLower(e[i].Format("")), strings.ToLower(cf.Formatted)) {
				return true
			}
		}
		return false
	}
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

func PrimarySuggestion(f *jqlpb.Filter) (string, bool) {
	// TODO implement
	return "", false
}

func Description(f *jqlpb.Filter) string {
	// TODO implement
	return ""
}

func Example(f *jqlpb.Filter) (int, string) {
	// TODO implement
	return 0, ""
}
