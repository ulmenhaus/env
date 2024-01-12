package api

import (
	"strings"

	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

type filterShim struct {
	filter *jqlpb.Filter
	colix  int
}

func xor(a, b bool) bool {
	return (a && !b) || (!a && b)
}

func (f *filterShim) Applies(e []types.Entry) bool {
	switch match := f.filter.Match.(type) {
	case *jqlpb.Filter_EqualMatch:
		return xor(e[f.colix].Format("") == match.EqualMatch.Value, f.filter.Negated)
	case *jqlpb.Filter_InMatch:
		// TODO really inefficient to construct this map every time
		asMap := slice2map(match.InMatch.Values)
		return asMap[e[f.colix].Format("")]
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

func (f *filterShim) Description() string {
	// TODO remove superfluous method
	return ""
}

func (f *filterShim) Example() (int, string) {
	// TODO remove superfluous method
	return 0, ""
}

func (f *filterShim) PrimarySuggestion() (string, bool) {
	// TODO remove superfluous method
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

func slice2map(slice []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range slice {
		m[s] = true
	}
	return m
}

