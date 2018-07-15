package types

import (
	"fmt"
	"sort"
)

// An Entry is an internal representation of a single column in a
// single row of a database
type Entry interface {
	// Format takes in a format string and returns the
	// user-presentable representation of the entry.
	// When given an emptry string, should return the canonical form
	// of the object.
	Format(fmt string) string
	// Compare should return true iff the privded Entry
	// is greater than the Entry whose method is being called.
	// Behavior is undefined if the two Entries are of different types
	Compare(entry interface{}) bool
	// Add returns a new Entry with the provided value added
	Add(addend interface{}) (Entry, error)
}

// A Table is a model of an unordered two-dimensional array of data
type Table struct {
	Columns []string
	Entries map[string][]Entry

	columnsByName map[string]int
	primary       int
}

// NewTable returns a new table given a list of columns
func NewTable(columns []string, entries map[string][]Entry, primary string) *Table {
	columnsByName := map[string]int{}
	for i, col := range columns {
		columnsByName[col] = i
	}
	return &Table{
		Columns: columns,
		Entries: entries,

		columnsByName: columnsByName,
		// XXX need to verify column is in table
		primary: columnsByName[primary],
	}
}

// A Database is a collection of named tables
type Database map[string]*Table

// A Filter is a decision function on Entries
// TODO this should filter on rows
type Filter func(Entry) bool

// Query takes in a set of filters, and the name of a column to order by
// as well as a bool which is true iff the ordering shoud be decending.
// It returns a sub-table of just the filtered items
func (t *Table) Query(filters []Filter, orderBy string, dec bool) ([][]Entry, error) {
	if len(filters) != 0 {
		return nil, fmt.Errorf("filtered querying not implemented")
	}

	entries := [][]Entry{}
	for _, row := range t.Entries {
		entries = append(entries, row)
	}
	xor := func(b1, b2 bool) bool { return (b1 || b2) && !(b1 && b2) }
	if orderBy != "" {
		col, ok := t.columnsByName[orderBy]
		if !ok {
			return nil, fmt.Errorf("Unknown column for ordering: %s", orderBy)
		}
		sort.Slice(entries, func(i, j int) bool {
			return xor(dec, entries[i][col].Compare(entries[j][col]))
		})
	}
	return entries, nil
}

// Insert adds a new row to the table
func (t *Table) Insert(pk string) error {
	// NOTE Insert needs to be gorouting safe
	return fmt.Errorf("not implemented")
}

// Update modifies a row
func (t *Table) Update(pk, field, value string) error {
	// NOTE Update needs to be gorouting safe
	return fmt.Errorf("not implemented")
}

// Primary returns the index of the primary key column of the table
func (t *Table) Primary() int {
	return t.primary
}
