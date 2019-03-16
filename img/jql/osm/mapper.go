package osm

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
)

const schemataTableName = "_schemata"

var (
	constructors = map[string]types.FieldValueConstructor{
		"string": types.NewString,
		"int":    types.NewInteger,
		"date":   types.NewDate,
		"enum":   types.NewEnum,
		"id":     types.NewID,
		"time":   types.NewTime,
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
func (osm *ObjectStoreMapper) Load(src io.Reader) (*types.Database, error) {
	// XXX needs refactor
	raw, err := osm.store.Read(src)
	if err != nil {
		return nil, err
	}
	schemata, ok := raw[schemataTableName]
	if !ok {
		return nil, fmt.Errorf("missing schema table")
	}
	// field index is non deterministic which will lead to random output
	// we could have another attribute for "_presentation" which contains
	// column order and widths
	fieldsByTable := map[string][]string{}
	primariesByTable := map[string]string{}
	constructorsByTable := map[string](map[string]types.FieldValueConstructor){}
	featuresByColumnByTable := map[string](map[string](map[string]interface{})){}
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
		if strings.HasPrefix(fieldType, "dynamic.") {
			// TODO implement dymanic columns
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
		var constructor types.FieldValueConstructor
		if strings.HasPrefix(fieldType, "foreign.") {
			// TODO(rabrams) double check scoping of this variable
			// also would be good to validate foriegn values
			table := fieldType[len("foreign."):]
			constructor = func(i interface{}, features map[string]interface{}) (types.Entry, error) {
				if features == nil {
					features = map[string]interface{}{}
				}
				features["table"] = table
				return types.NewForeignKey(i, features)
			}
		} else {
			constructor, ok = constructors[fieldType]
			if !ok {
				return nil, fmt.Errorf("invalid type '%s'", fieldType)
			}
		}
		byTable, ok := fieldsByTable[table]
		if !ok {
			fieldsByTable[table] = []string{column}
			constructorsByTable[table] = map[string]types.FieldValueConstructor{
				column: constructor,
			}
			featuresByColumnByTable[table] = map[string](map[string]interface{}){}
		} else {
			fieldsByTable[table] = append(byTable, column)
			constructorsByTable[table][column] = constructor
		}
		features := map[string]interface{}{}
		featuresUncast, ok := schema["features"]
		if ok {
			features, ok = featuresUncast.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid type for `features`")
			}
		}
		featuresByColumnByTable[table][column] = features
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

	delete(raw, schemataTableName)
	db := &types.Database{
		Schemata: schemata,
		Tables:   map[string]*types.Table{},
	}
	for name, encoded := range raw {
		primary, ok := primariesByTable[name]
		if !ok {
			return nil, fmt.Errorf("Unknown table: %s", name)
		}
		// TODO use a constructor and Inserts -- that way the able can map
		// columns by name
		table := types.NewTable(fieldsByTable[name], map[string][]types.Entry{}, primary, constructorsByTable[name], featuresByColumnByTable[name])
		allFields := fieldsByTable[name]

		db.Tables[name] = table
		for pk, fields := range encoded {
			row := make([]types.Entry, len(fieldsByTable[name]))
			table.Entries[pk] = row
			fields[primary] = pk
			for _, column := range allFields {
				value := fields[column]
				fullName := fmt.Sprintf("%s.%s", name, column)
				index, ok := indexMap[fullName]
				if !ok {
					return nil, fmt.Errorf("unknown column: %s", fullName)
				}
				constructor := constructorsByTable[name][column]

				typedVal, err := constructor(value, featuresByColumnByTable[name][column])
				if err != nil {
					return nil, fmt.Errorf("failed to init %s.%s for %s: %s", name, column, pk, err)
				}
				row[index] = typedVal
			}
		}
	}
	return db, nil
}

// Dump takes the database and serializes it using the storage driver
func (osm *ObjectStoreMapper) Dump(db *types.Database, dst io.Writer) error {
	encoded := storage.EncodedDatabase{
		schemataTableName: db.Schemata,
	}
	for name, table := range db.Tables {
		encodedTable := storage.EncodedTable{}
		pkCol := table.Primary()
		for pk, row := range table.Entries {
			// TODO inconsistent use of entry in types and storage
			encodedEntry := storage.EncodedEntry{}
			for i, entry := range row {
				if i != pkCol {
					encodedEntry[table.Columns[i]] = entry.Encoded()
				}
			}
			encodedTable[pk] = encodedEntry
		}
		encoded[name] = encodedTable
	}
	err := osm.store.Write(dst, encoded)
	return err
}
