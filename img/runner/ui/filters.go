package ui

import (
	"strings"

	"github.com/ulmenhaus/env/img/jql/types"
)

type nounFilter struct {
	nouns map[string]bool
	col   int
}

func (nf *nounFilter) Applies(entries []types.Entry) bool {
	entry := entries[nf.col].Format("")
	if !strings.HasPrefix(entry, PrefixNouns) {
		return false
	}
	arg := entry[len(PrefixNouns):]
	return nf.nouns[arg]
}

func (nf *nounFilter) Description() string {
	return ""
}

func (nf *nounFilter) Example() (int, string) {
	return -1, ""
}

func (nf *nounFilter) PrimarySuggestion() (string, bool) {
	return "", false
}
