package ui

import (
	"fmt"

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
	return fmt.Sprintf("%s = %s", ef.Field, ef.Formatted)
}

func (ef *EqualFilter) Example() (int, string) {
	return ef.Col, ef.Formatted
}
