package api

import (
	"fmt"
	"strings"

	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

type filterShim struct {
	filter *jqlpb.Filter
	colix  int
	asMap  map[string]bool
}

func xor(a, b bool) bool {
	return (a && !b) || (!a && b)
}

func (f *filterShim) init() {
	switch match := f.filter.Match.(type) {
	case *jqlpb.Filter_InMatch:
		// TODO really inefficient to construct this map every time
		f.asMap = slice2map(match.InMatch.Values)
	}
}

func (f *filterShim) Applies(e []types.Entry) bool {
	switch match := f.filter.Match.(type) {
	case *jqlpb.Filter_EqualMatch:
		return xor(e[f.colix].Format("") == match.EqualMatch.Value, f.filter.Negated)
	case *jqlpb.Filter_InMatch:
		return f.asMap[e[f.colix].Format("")]
	case *jqlpb.Filter_ContainsMatch:
		cm := match.ContainsMatch
		// NOTE exact match + col < 0 not implemented and will cause a panic
		if cm.Exact {
			// HACK to make a ContainsFilter work for ForeignLists format to the full list.
			// NOTE this behavior is only partially correct as it  relies
			// on keys not having newlines
			return strings.Contains(e[f.colix].Format(types.ListFormat), "\n"+cm.Value+"\n")

		}
		if f.colix < 0 {
			for i := 0; i < len(e); i++ {
				if strings.Contains(strings.ToLower(e[i].Format("")), strings.ToLower(cm.Value)) {
					return true
				}
			}
			return false
		}
		return strings.Contains(strings.ToLower(e[f.colix].Format("")), strings.ToLower(cm.Value))

	}
	return false
}

// PrimarySuggestion returns a suggestion for prefilling the primary key of a new
// entry when this filter is applied as well as a boolean which may be false if the
// filter has no suggestion
func PrimarySuggestion(f *jqlpb.Filter) (string, bool) {
	switch match := f.Match.(type) {
	case *jqlpb.Filter_EqualMatch:
		if f.Negated {
			return "", false
		}
		return match.EqualMatch.Value, true
	case *jqlpb.Filter_InMatch:
		return "", false
	case *jqlpb.Filter_ContainsMatch:
		return "", false
	}
	return "", false
}

// Description returns a user-facing description of the Filter
func Description(f *jqlpb.Filter) string {
	switch match := f.Match.(type) {
	case *jqlpb.Filter_EqualMatch:
		op := "="
		if f.Negated {
			op = "!="
		}
		return fmt.Sprintf("%s %s \"%s\"", f.Column, op, strings.Replace(match.EqualMatch.Value, "\"", "\\\"", -1))
	case *jqlpb.Filter_InMatch:
		return fmt.Sprintf("%s in (%s)", f.Column, strings.Join(match.InMatch.Values, ", "))
	case *jqlpb.Filter_ContainsMatch:
		return fmt.Sprintf("%s contains \"%s\"", f.Column, strings.Replace(match.ContainsMatch.Value, "\"", "\\\"", -1))
	}
	return ""
}

// Example returns a column and an example formatted value that would match the
// given filter or -1 if no such matching is possible
func Example(columns []*jqlpb.Column, f *jqlpb.Filter) (int, string) {
	col := IndexOfField(columns, f.GetColumn())
	switch match := f.Match.(type) {
	case *jqlpb.Filter_EqualMatch:
		if f.Negated {
			return col, ""
		}
		return col, match.EqualMatch.Value
	case *jqlpb.Filter_InMatch:
		return col, match.InMatch.Values[0]
	case *jqlpb.Filter_ContainsMatch:
		return col, match.ContainsMatch.Value
	}
	return 0, ""
}

func slice2map(slice []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range slice {
		m[s] = true
	}
	return m
}
