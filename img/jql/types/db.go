package types

import "fmt"

// An Entry is an internal representation of a single column in a
// single row of a database
type Entry interface {
	// Format takes in a format string and returns the
	// user-presentable representation of the entry
	Format(fmt string) string
}

// A Table is a model of an unordered two-dimensional array of data
type Table struct {
	Columns []string
	Entries map[string][]Entry
}

// A Database is a collection of named tables
type Database map[string]Table

// A Filter is a decision function on Entries
// TODO this should filter on rows
type Filter func(Entry) bool

// Query takes in a set of filters, and the name of a column to order by
// It returns a sub-table of just the filtered items
func (t *Table) Query(filters []Filter, orderBy string) (*Table, error) {
	if len(filters) != 0 || orderBy != "" {
		return nil, fmt.Errorf("table querying not implemented")
	}
	return t, nil
}

// Insert adds a new row to the table
func (t *Table) Insert(pk string) error {
	return fmt.Errorf("not implemented")
}

// Update modifies a row
func (t *Table) Update(pk, field, value string) error {
	return fmt.Errorf("not implemented")
}
