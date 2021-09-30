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

type eqFilter struct {
	col int
	val string
}

func (ef *eqFilter) Applies(entries []types.Entry) bool {
	entry := entries[ef.col].Format("")
	return entry == ef.val
}

func (ef *eqFilter) Description() string {
	return ""
}

func (ef *eqFilter) Example() (int, string) {
	return -1, ""
}

func (ef *eqFilter) PrimarySuggestion() (string, bool) {
	return "", false
}

type inFilter struct {
	col int
	val string
}

func (inf *inFilter) Applies(entries []types.Entry) bool {
	entry := entries[inf.col].Format("")
	return strings.Contains(strings.ToLower(entry), strings.ToLower(inf.val))
}

func (inf *inFilter) Description() string {
	return ""
}

func (inf *inFilter) Example() (int, string) {
	return -1, ""
}

func (inf *inFilter) PrimarySuggestion() (string, bool) {
	return "", false
}
