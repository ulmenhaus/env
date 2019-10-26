package types

import (
	"fmt"

	"github.com/ulmenhaus/env/img/jql/storage"
)

const (
	ListFormat = "<list>"
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
	if ft == ListFormat {
		return "\n" + fk.Key + "\n"
	}
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

// A ForeignList stores a collection of primary keys for another entry in
// another table. ForeignLists are useful for modeling many-to-many relationships
// between entries in tables.
type ForeignList struct {
	Table string
	Keys  []string
}

// NewForeignList returns a new Entry from the encoded data
func NewForeignList(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		i = []string{""}
	}
	keysI, ok := i.([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to unpack slice from: %#v", i)
	}
	keys := []string{}
	for _, keyI := range keysI {
		key, ok := keyI.(string)
		if !ok {
			return nil, fmt.Errorf("failed to unpack string from: %#v", i)
		}
		keys = append(keys, key)
	}
	table, ok := features["table"]
	if !ok {
		return nil, fmt.Errorf("table not provided for foerign key")
	}
	tableName, ok := table.(string)
	if !ok {
		return nil, fmt.Errorf("table must be a string")
	}
	return ForeignList{
		Table: tableName,
		Keys:  keys,
	}, nil
}

// Format formats the key
func (fl ForeignList) Format(ft string) string {
	if ft == ListFormat {
		fullList := "\n"
		for _, key := range fl.Keys {
			fullList = fullList + key + "\n"
		}
		return fullList
	}
	return fmt.Sprintf("%d refs", len(fl.Keys))
}

// Reverse creates a new key from the input
func (fl ForeignList) Reverse(ft, input string) (Entry, error) {
	return nil, fmt.Errorf("Reversing ForeignLists not supported")
}

// Compare returns true iff the given object is a foreign key
// that comes lexicographically after this one
func (fl ForeignList) Compare(i interface{}) bool {
	entry, ok := i.(ForeignList)
	if !ok {
		return false
	}
	return len(entry.Keys) > len(fl.Keys)
}

// Add adds to the foreign key
func (fl ForeignList) Add(i interface{}) (Entry, error) {
	return nil, fmt.Errorf("Cannot add to a foreign list")
}

// Encoded returns the ForeignList encoded as a int
func (fl ForeignList) Encoded() storage.Primitive {
	return fl.Keys
}
