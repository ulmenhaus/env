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
	// TODO could just use constructor for this instead
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

// A FieldValueConstructor is a function which takes in a base encoded
// version of the entry and returns the entry itself. If given nil
// the function should return a reasonable default value.
type FieldValueConstructor func(encoded interface{}, features map[string]interface{}) (Entry, error)

// A Table is a model of an unordered two-dimensional array of data
type Table struct {
	Columns []string
	Entries map[string][]Entry

	columnsByName    map[string]int
	primary          int
	Constructors     map[string]FieldValueConstructor
	featuresByColumn map[string](map[string]interface{})
}

// NewTable returns a new table given a list of columns
func NewTable(columns []string, entries map[string][]Entry, primary string, constructors map[string]FieldValueConstructor, featuresByColumn map[string](map[string]interface{})) *Table {
	columnsByName := map[string]int{}
	for i, col := range columns {
		columnsByName[col] = i
	}
	return &Table{
		Columns: columns,
		Entries: entries,

		columnsByName: columnsByName,
		// XXX need to verify column is in table
		primary:          columnsByName[primary],
		Constructors:     constructors,
		featuresByColumn: featuresByColumn,
	}
}

// A Database is a collection of named tables
type Database struct {
	// TODO remove dependency on storage package, perhaps by storing
	// schemata as an actual table
	Schemata storage.EncodedTable
	Tables   map[string]*Table
}

// A Filter reduces the set of Entries to just those the user is interested in
// seeing at a given time
type Filter interface {
	// Applies returns true iff the provided entry should be shown given the filter
	Applies([]Entry) bool
	// Description returns a user-facing description of the Filter
	Description() string
	// Example returns a column and an example formatted value that would match the
	// given filter or -1 if no such matching is possible
	Example() (int, string)
	// PrimarySuggestion returns a suggestion for prefilling the primary key of a new
	// entry when this filter is applied as well as a boolean which may be false if the
	// filter has no suggestion
	PrimarySuggestion() (string, bool)
}

// QueryParams are the parameters to a table's Query method
type QueryParams struct {
	// Filters are the filters to apply (conjunctively) to
	// the rows
	Filters []Filter
	// OrderBy is the name of the field by which to order the data
	OrderBy string
	// Dec is true iff the order should be decending
	Dec bool
	// Offset is the ordinal of the row from which the response should start
	Offset uint
	// Limit is the max number of entries the query should return -- if 0
	// the response will be uncapped
	Limit uint
}

// A Response is a paginated collection of entries that match a query
type Response struct {
	Entries [][]Entry
	Total   uint
}

// IntMin returns the min of two integers
func IntMin(a, b uint) uint {
	if a < b {
		return a
	}
	return b
}

// Query takes in a set of filters, and the name of a column to order by
// as well as a bool which is true iff the ordering shoud be decending.
// It returns a sub-table of just the filtered items
func (t *Table) Query(params QueryParams) (*Response, error) {
	entries := [][]Entry{}
	for _, row := range t.Entries {
		out := false
		for _, filter := range params.Filters {
			if !filter.Applies(row) {
				out = true
			}
		}
		if !out {
			entries = append(entries, row)
		}
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
	total := uint(len(entries))
	cap := total
	if params.Limit != 0 {
		cap = IntMin(params.Offset+params.Limit, cap)
	}
	entries = entries[params.Offset:cap]
	resp := &Response{
		Entries: entries,
		Total:   total,
	}
	return resp, nil
}

// Insert adds a new row to the table
func (t *Table) Insert(pk string) error {
	// TODO Insert needs to be gorouting safe
	_, ok := t.Entries[pk]
	if ok {
		return fmt.Errorf("Row already exists with pk '%s'", pk)
	}
	row := []Entry{}
	for i, col := range t.Columns {
		constructor := t.Constructors[col]
		var input interface{}
		if i == t.primary {
			input = pk
		}
		entry, err := constructor(input, t.featuresByColumn[t.Columns[i]])
		if err != nil {
			return err
		}
		row = append(row, entry)
	}
	t.Entries[pk] = row
	return nil
}

func (t *Table) InsertWithFields(pk string, fields map[string]string) error {
	err := t.Insert(pk)
	if err != nil {
		return err
	}
	for field, value := range fields {
		err = t.Update(pk, field, value)
		if err != nil {
			return err
		}
	}
	return nil
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

// Delete removes a row
func (t *Table) Delete(pk string) error {
	_, ok := t.Entries[pk]
	if !ok {
		return fmt.Errorf("Row does not exist with pk %s", pk)
	}
	delete(t.Entries, pk)
	return nil
}

// HasForeign returns the index of the column that is a foriegn key to the
// provided table or -1 if there is no such column
func (t *Table) HasForeign(table, formatted string) int {
	// FIXME leaky abstraction, inefficient, and not guaranteed to be correct

	// Take a first pass and try to return a column that has entries. This means
	// that a table that has multiple foreign keys to the same table will prefer
	// a column that's used
	for name, features := range t.featuresByColumn {
		if foreign, ok := features["table"]; ok && table == foreign {
			col := t.columnsByName[name]
			// FIXME this is super inefficent. Would be nice to keep an index mapping
			// col value to entries
			for _, entry := range t.Entries {
				if entry[col].Format("") == formatted {
					return col
				}
			}
		}
	}
	// Fall back to using any column
	for name, features := range t.featuresByColumn {
		if foreign, ok := features["table"]; ok && table == foreign {
			return t.columnsByName[name]
		}
	}
	return -1
}

// IndexOfField returns the index of a column given the name of that column
// If the column does not exist, -1 is returned
func (t *Table) IndexOfField(field string) int {
	index, ok := t.columnsByName[field]
	if !ok {
		return -1
	}
	return index
}
