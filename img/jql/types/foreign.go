package types

import (
	"fmt"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// A ForeignKey stores the primary key for another entry in another table
type ForeignKey struct {
	Table string
	Key   string
}

// NewForeignKey returns a new string from the encoded data
func NewForeignKey(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		i = ""
	}
	key, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("failed to unpack string from: %#v", i)
	}
	table, ok := features["table"]
	if !ok {
		return nil, fmt.Errorf("table not provided for foerign key")
	}
	tableName, ok := table.(string)
	if !ok {
		return nil, fmt.Errorf("table must be a string")
	}
	return ForeignKey{
		Table: tableName,
		Key:   key,
	}, nil
}

// Format formats the key
func (fk ForeignKey) Format(ft string) string {
	return fk.Key
}

// Reverse creates a new key from the input
func (fk ForeignKey) Reverse(ft, input string) (Entry, error) {
	return ForeignKey{
		Table: fk.Table,
		Key:   input,
	}, nil
}

// Compare returns true iff the given object is a foreign key
// that comes lexicographically after this one
func (fk ForeignKey) Compare(i interface{}) bool {
	entry, ok := i.(ForeignKey)
	if !ok {
		return false
	}
	return entry.Key > fk.Key
}

// Add adds to the foreign key
func (fk ForeignKey) Add(i interface{}) (Entry, error) {
	return nil, fmt.Errorf("Cannot add to a foreign key")
}

// Encoded returns the ForeignKey encoded as a int
func (fk ForeignKey) Encoded() storage.Primitive {
	return fk.Key
}
