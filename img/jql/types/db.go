package types

import (
	"fmt"
	"sort"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// An Entry is an internal representation of a single column in a
// single row of a database
type Entry interface {
	// Format takes in a format string and returns the
	// user-presentable representation of the entry.
	// When given an emptry string, should return the canonical form
	// of the object.
	Format(fmt string) string
	// Reverse takes in a format string and an input and
	// returns a new Entry whose representation with the given
	// format should be the input
	Reverse(fmt, input string) (Entry, error)
	// Compare should return true iff the privded Entry
	// is greater than the Entry whose method is being called.
	// Behavior is undefined if the two Entries are of different types
	Compare(entry interface{}) bool
	// Add returns a new Entry with the provided value added
	Add(addend interface{}) (Entry, error)
	// Encoded returns the entry encoded as a primitive
	// TODO remove dependency on storage package
	Encoded() storage.Primitive
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
type Database struct {
	// TODO remove dependency on storage package, perhaps by storing
	// schemata as an actual table
	Schemata storage.EncodedTable
	Tables   map[string]*Table
}

// A Filter is a decision function on Entries
// TODO this should filter on rows
type Filter func(Entry) bool

// QueryParams are the parameters to a table's Query method
type QueryParams struct {
	// Filters are the filters to apply (conjunctively) to
	// the rows
	Filters []Filter
	// OrderBy is the name of the field by which to order the data
	OrderBy string
	// Dec is true iff the order should be decending
	Dec bool
}

// Query takes in a set of filters, and the name of a column to order by
// as well as a bool which is true iff the ordering shoud be decending.
// It returns a sub-table of just the filtered items
func (t *Table) Query(params QueryParams) ([][]Entry, error) {
	if len(params.Filters) != 0 {
		return nil, fmt.Errorf("filtered querying not implemented")
	}

	entries := [][]Entry{}
	for _, row := range t.Entries {
		entries = append(entries, row)
	}
	xor := func(b1, b2 bool) bool { return (b1 || b2) && !(b1 && b2) }
	if params.OrderBy != "" {
		col, ok := t.columnsByName[params.OrderBy]
		if !ok {
			return nil, fmt.Errorf("Unknown column for ordering: %s", params.OrderBy)
		}
		sort.Slice(entries, func(i, j int) bool {
			return xor(params.Dec, entries[i][col].Compare(entries[j][col]))
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
	col, ok := t.columnsByName[field]
	if !ok {
		return fmt.Errorf("Unknown column: %s", field)
	}
	current, ok := t.Entries[pk]
	if !ok {
		return fmt.Errorf("Row does not exist with pk %s", pk)
	}
	// TODO this needs to be passed the format string
	new, err := current[col].Reverse("", value)
	if err != nil {
		return err
	}
	current[col] = new
	if col == t.primary {
		delete(t.Entries, pk)
		t.Entries[new.Format("")] = current
	}
	return nil
}

// Primary returns the index of the primary key column of the table
func (t *Table) Primary() int {
	return t.primary
}
