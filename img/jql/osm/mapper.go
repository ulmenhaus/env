package osm

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
)

type fieldValueConstructor func(interface{}) (types.Entry, error)

var (
	constructors = map[string]fieldValueConstructor{
		"string": types.NewString,
	}
)

// An ObjectStoreMapper is responsible for converting between the
// internal representation of a database and the encoded version
// used by storage drivers
type ObjectStoreMapper struct {
	store storage.Store
}

// NewObjectStoreMapper returns a new ObjectStoreMapper given a storage driver
func NewObjectStoreMapper(store storage.Store) (*ObjectStoreMapper, error) {
	return &ObjectStoreMapper{
		store: store,
	}, nil
}

// Load takes the given reader of a serialized databse and returns a databse object
func (osm *ObjectStoreMapper) Load(src io.Reader) (types.Database, error) {
	// XXX needs refactor
	raw, err := osm.store.Read(src)
	if err != nil {
		return nil, err
	}
	schemata, ok := raw["_schemata"]
	if !ok {
		return nil, fmt.Errorf("missing schema table")
	}
	// field index is non deterministic which will lead to random output
	// we could have another attribute for "_presentation" which contains
	// column order and widths
	fieldsByTable := map[string][]string{}
	primariesByTable := map[string]string{}
	for name, schema := range schemata {
		parts := strings.Split(name, ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid column name: %s", name)
		}
		table := parts[0]
		column := parts[1]
		// TODO schema validation outside of loop -- this is inefficient
		fieldTypeRaw, ok := schema["type"]
		if !ok {
			return nil, fmt.Errorf("missing type for %s.%s", table, column)
		}
		fieldType, ok := fieldTypeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("invalid type %#v", fieldTypeRaw)
		}
		if strings.HasPrefix(fieldType, "foreign.") || strings.HasPrefix(fieldType, "dynamic.") {
			// TODO implement foreign keys and dymanic columns
			// ignoring for now
			continue
		}
		if primary, ok := schema["primary"]; ok {
			if primaryB, ok := primary.(bool); ok && primaryB {
				if currentPrimary, ok := primariesByTable[table]; ok {
					return nil, fmt.Errorf("Duplicate primary keys for %s: %s %s", table, currentPrimary, column)
				}
				primariesByTable[table] = column
			}
		}
		byTable, ok := fieldsByTable[table]
		if !ok {
			fieldsByTable[table] = []string{column}
		} else {
			fieldsByTable[table] = append(byTable, column)
		}
	}

	indexMap := map[string]int{}
	for table, byTable := range fieldsByTable {
		sort.Slice(byTable, func(i, j int) bool { return byTable[i] < byTable[j] })
		for index, column := range byTable {
			indexMap[fmt.Sprintf("%s.%s", table, column)] = index
		}
		if _, ok := primariesByTable[table]; !ok {
			return nil, fmt.Errorf("No primary key for table: %s", table)
		}
	}

	delete(raw, "_schemata")
	db := types.Database{}
	for name, encoded := range raw {
		table := types.Table{
			// TODO use a constructor and Inserts -- that way the able can map
			// columns by name
			Columns: fieldsByTable[name],
			Entries: map[string][]types.Entry{},
		}
		db[name] = table
		primary, ok := primariesByTable[name]
		if !ok {
			return nil, fmt.Errorf("Unknown table: %s", name)
		}
		for pk, fields := range encoded {
			row := make([]types.Entry, len(fieldsByTable[name]))
			table.Entries[pk] = row
			fields[primary] = pk
			for column, value := range fields {
				fullName := fmt.Sprintf("%s.%s", name, column)
				index, ok := indexMap[fullName]
				if !ok {
					return nil, fmt.Errorf("unknown column: %s", fullName)
				}
				schema, ok := schemata[fullName]
				if !ok {
					return nil, fmt.Errorf("missing schema for %s.%s", name, column)
				}
				// TODO use structured data from above schema validation
				// instead of keying map
				fieldType := schema["type"].(string)
				constructor, ok := constructors[fieldType]
				if !ok {
					return nil, fmt.Errorf("invalid type '%s'", fieldType)
				}
				typedVal, err := constructor(value)
				if err != nil {
					return nil, err
				}
				row[index] = typedVal
			}
		}
	}
	return db, nil
}
