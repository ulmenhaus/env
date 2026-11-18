package osm

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ulmenhaus/env/img/jql/storage"
)

const (
	// implicitPKColumn is the synthesized primary key column for tables with an implicit schema
	implicitPKColumn = "_row"
	// implicitValueColumn holds rows that are not objects
	implicitValueColumn = "value"
)

// inferEncodedDB reads a plain JSON document and converts it into an encoded database
// with a synthesized schemata table. The document may be either
//
//  1. a map from table name to a list of objects, producing one table per key, or
//  2. a list of objects, producing a single table named after the file
//
// Each object maps column names to values. Strings become string columns, numbers become
// int columns (or number columns if any value is fractional) and everything else is
// converted to a string.
func inferEncodedDB(src io.Reader, path string) (storage.EncodedDatabase, error) {
	b, err := ioutil.ReadAll(src)
	if err != nil {
		return nil, err
	}
	var doc interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}

	rowsByTable := map[string][]interface{}{}
	switch typed := doc.(type) {
	case []interface{}:
		base := filepath.Base(path)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		rowsByTable[sanitizeName(name)] = typed
	case map[string]interface{}:
		for name, rows := range typed {
			list, ok := rows.([]interface{})
			if !ok {
				return nil, fmt.Errorf("expected a list of objects for type %q but got %T", name, rows)
			}
			rowsByTable[sanitizeName(name)] = list
		}
	default:
		return nil, fmt.Errorf("implicit schema requires a top-level JSON list or map, got %T", doc)
	}

	raw := storage.EncodedDatabase{}
	schemata := storage.EncodedTable{}
	for name, rows := range rowsByTable {
		if name == schemataTableName {
			return nil, fmt.Errorf("table name %q is reserved", name)
		}
		table, columnTypes := inferTable(rows)
		raw[name] = table
		for column, ftype := range columnTypes {
			schemata[fmt.Sprintf("%s.%s", name, column)] = storage.EncodedEntry{"type": ftype}
		}
		schemata[fmt.Sprintf("%s.%s", name, implicitPKColumn)] = storage.EncodedEntry{
			"type":    "string",
			"primary": true,
		}
	}
	raw[schemataTableName] = schemata
	return raw, nil
}

// inferTable converts a list of objects into an encoded table and returns
// the inferred type of each column
func inferTable(rows []interface{}) (storage.EncodedTable, map[string]string) {
	objects := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		obj, ok := row.(map[string]interface{})
		if !ok {
			obj = map[string]interface{}{implicitValueColumn: row}
		}
		sanitized := map[string]interface{}{}
		for column, value := range obj {
			column = sanitizeName(column)
			if column == implicitPKColumn {
				continue
			}
			sanitized[column] = value
		}
		objects[i] = sanitized
	}

	// Determine the type of each column: a column is numeric only if
	// all of its non-null values are numbers
	columnTypes := map[string]string{}
	for _, obj := range objects {
		for column, value := range obj {
			current, seen := columnTypes[column]
			var valueType string
			switch v := value.(type) {
			case nil:
				if !seen {
					columnTypes[column] = ""
				}
				continue
			case float64:
				if v == math.Trunc(v) && math.Abs(v) < math.MaxInt64 {
					valueType = "int"
				} else {
					valueType = "number"
				}
			default:
				valueType = "string"
			}
			switch {
			case current == "" || current == valueType:
				columnTypes[column] = valueType
			case (current == "int" && valueType == "number") || (current == "number" && valueType == "int"):
				columnTypes[column] = "number"
			default:
				columnTypes[column] = "string"
			}
		}
	}
	for column, ftype := range columnTypes {
		if ftype == "" {
			columnTypes[column] = "string"
		}
	}

	width := len(strconv.Itoa(len(objects)))
	table := storage.EncodedTable{}
	for i, obj := range objects {
		entry := storage.EncodedEntry{}
		for column, value := range obj {
			if value == nil {
				continue
			}
			if columnTypes[column] == "string" {
				value = stringify(value)
			}
			entry[column] = value
		}
		table[fmt.Sprintf("%0*d", width, i)] = entry
	}
	return table, columnTypes
}

// stringify converts an arbitrary JSON value into a string
func stringify(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// sanitizeName ensures a table or column name doesn't contain the separator
// used for schemata keys
func sanitizeName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}
