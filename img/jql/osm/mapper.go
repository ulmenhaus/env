package osm

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

const (
	schemataTableName = "_schemata" // the name of the table containing schemata for other tables
)

var (
	constructors = map[string]types.FieldValueConstructor{
		"string":   types.NewString,
		"int":      types.NewInteger,
		"date":     types.NewDate,
		"enum":     types.NewEnum,
		"id":       types.NewID,
		"time":     types.NewTime,
		"moneyamt": types.NewMoneyAmount,
	} // maps field types to their corresponding constructor functions
	fieldTypes = map[string]jqlpb.EntryType{
		"string":   jqlpb.EntryType_STRING,
		"int":      jqlpb.EntryType_INT,
		"date":     jqlpb.EntryType_DATE,
		"enum":     jqlpb.EntryType_ENUM,
		"id":       jqlpb.EntryType_ID,
		"time":     jqlpb.EntryType_TIME,
		"moneyamt": jqlpb.EntryType_MONEYAMT,
	}
)

// An ObjectStoreMapper is responsible for converting between the
// internal representation of a database and the encoded version
// used by storage drivers
type ObjectStoreMapper struct {
	store storage.Store // the storage.Store to which items are stored
	path  string

	// NOTE to support an incremental migration to daemonized jql we store
	// the database as an attribute on the OSM that can be exposed to
	// higher level callers. Once exposure to the database is fully
	// hidden behind the DBMS API and we are doing sharded storage
	// we can reconsider the handoff between the OSM and the API layer
	db *types.Database

	mu      sync.Mutex
	updates map[GlobalKey]string // if nil a snapshot was loaded and we update everything
}

// NewObjectStoreMapper returns a new ObjectStoreMapper given a storage driver
func NewObjectStoreMapper(path string) (*ObjectStoreMapper, error) {
	var store storage.Store
	if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".jql") {
		store = &storage.JSONStore{}
	} else {
		return nil, fmt.Errorf("Unknown file type")
	}
	return &ObjectStoreMapper{
		store:   store,
		path:    path,
		updates: map[GlobalKey]string{},
	}, nil
}

func (osm *ObjectStoreMapper) getAndPurgeUpdates() map[GlobalKey]string {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	updates := osm.updates
	osm.updates = map[GlobalKey]string{}
	return updates
}

func (osm *ObjectStoreMapper) AllUpdated() {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	osm.updates = nil
}

func (osm *ObjectStoreMapper) RowUpdating(tname, pk string) {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	if osm.updates != nil {
		shardKey := ""
		table := osm.db.Tables[tname]
		strategy := getShardStrategy(table)
		if strategy.shardBy != "" {
			shardKey = table.Entries[pk][table.IndexOfField(strategy.shardBy)].Format("")
		}
		osm.updates[GlobalKey{Table: tname, PK: pk}] = shardKey
	}
}

func (osm *ObjectStoreMapper) GetDB() *types.Database {
	return osm.db
}

func (osm *ObjectStoreMapper) Load() error {
	if strings.HasSuffix(osm.path, ".json") {
		f, err := os.Open(osm.path)
		if err != nil {
			return err
		}
		defer f.Close()
		return osm.LoadSnapshot(f)
	} else if strings.HasSuffix(osm.path, ".jql") {
		return osm.loadDirectory()
	} else {
		return fmt.Errorf("unkown file type")
	}
}

func (osm *ObjectStoreMapper) readShard(path string) (storage.EncodedTable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return osm.store.ReadShard(f)
}

func (osm *ObjectStoreMapper) loadDirectory() error {
	raw := storage.EncodedDatabase{}
	var paths []string
	err := filepath.Walk(osm.path, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".json") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, path := range paths {
		shard, err := osm.readShard(path)
		if err != nil {
			return err
		}

		relpath, err := filepath.Rel(osm.path, path)
		if err != nil {
			return err
		}
		parts := strings.Split(relpath, string(os.PathSeparator))
		table := strings.Split(parts[0], ".")[0]

		if _, ok := raw[table]; !ok {
			raw[table] = storage.EncodedTable{}
		}

		for pk, value := range shard {
			raw[table][pk] = value
		}
	}
	return osm.loadEncodedDB(raw)
}

// Load takes the given reader of a serialized databse and returns a databse object
func (osm *ObjectStoreMapper) LoadSnapshot(src io.Reader) error {
	raw, err := osm.store.Read(src)
	if err != nil {
		return err
	}
	return osm.loadEncodedDB(raw)
}

func (osm *ObjectStoreMapper) loadEncodedDB(raw storage.EncodedDatabase) error {
	// XXX needs refactor
	schemata, ok := raw[schemataTableName]
	if !ok {
		return fmt.Errorf("missing schema table")
	}
	// field index is non deterministic which will lead to random output
	// we could have another attribute for "_presentation" which contains
	// column order and widths
	fieldsByTable := map[string][]string{}
	primariesByTable := map[string]string{}
	constructorsByTable := map[string](map[string]types.FieldValueConstructor){}
	featuresByColumnByTable := map[string](map[string](map[string]interface{})){}
	columnMetaByTable := map[string](map[string]*types.ColumnMeta){}
	for name, schema := range schemata {
		parts := strings.Split(name, ".")
		if len(parts) != 2 {
			return fmt.Errorf("invalid column name: %s", name)
		}
		table := parts[0]
		column := parts[1]
		// TODO schema validation outside of loop -- this is inefficient
		fieldTypeRaw, ok := schema["type"]
		if !ok {
			return fmt.Errorf("missing type for %s.%s", table, column)
		}
		fieldType, ok := fieldTypeRaw.(string)
		if !ok {
			return fmt.Errorf("invalid type %#v", fieldTypeRaw)
		}
		if strings.HasPrefix(fieldType, "dynamic.") {
			// TODO implement dymanic columns
			// ignoring for now
			continue
		}
		if primary, ok := schema["primary"]; ok {
			if primaryB, ok := primary.(bool); ok && primaryB {
				if currentPrimary, ok := primariesByTable[table]; ok {
					return fmt.Errorf("Duplicate primary keys for %s: %s %s", table, currentPrimary, column)
				}
				primariesByTable[table] = column
			}
		}
		var constructor types.FieldValueConstructor
		var entryType jqlpb.EntryType
		var foreignTable string
		var values []string
		if strings.HasPrefix(fieldType, "foreign.") {
			// TODO(rabrams) double check scoping of this variable
			// also would be good to validate foriegn values
			table := fieldType[len("foreign."):]
			entryType = jqlpb.EntryType_FOREIGN
			foreignTable = table
			constructor = func(i interface{}, features map[string]interface{}) (types.Entry, error) {
				if features == nil {
					features = map[string]interface{}{}
				}
				features["table"] = table
				return types.NewForeignKey(i, features)
			}

		} else if strings.HasPrefix(fieldType, "foreigns.") {
			// TODO(rabrams) double check scoping of this variable
			// also would be good to validate foriegn values
			table := fieldType[len("foreigns."):]
			entryType = jqlpb.EntryType_FOREIGNS
			foreignTable = table
			constructor = func(i interface{}, features map[string]interface{}) (types.Entry, error) {
				if features == nil {
					features = map[string]interface{}{}
				}
				features["table"] = table
				return types.NewForeignList(i, features)
			}
		} else {
			constructor, ok = constructors[fieldType]
			if !ok {
				return fmt.Errorf("invalid type '%s'", fieldType)
			}
			entryType, ok = fieldTypes[fieldType]
			if !ok {
				return fmt.Errorf("invalid type '%s'", fieldType)
			}
		}
		byTable, ok := fieldsByTable[table]
		var primaryShards int
		if shard, ok := schema["primary_shards"]; ok {
			if asInt, ok := shard.(float64); ok {
				primaryShards = int(asInt)
			} else {
				return fmt.Errorf("invalid type for primary shards: %T", shard)
			}
		}
		var secondaryShards int
		if shard, ok := schema["secondary_shards"]; ok {
			if asInt, ok := shard.(float64); ok {
				secondaryShards = int(asInt)
			} else {
				return fmt.Errorf("invalid type for secondary shards: %T", shard)
			}
		}
		meta := &types.ColumnMeta{
			Type:            entryType,
			ForeignTable:    foreignTable,
			Values:          values,
			PrimaryShards:   primaryShards,
			SecondaryShards: secondaryShards,
		}
		if !ok {
			fieldsByTable[table] = []string{column}
			constructorsByTable[table] = map[string]types.FieldValueConstructor{
				column: constructor,
			}
			featuresByColumnByTable[table] = map[string](map[string]interface{}){}
			columnMetaByTable[table] = map[string]*types.ColumnMeta{column: meta}
		} else {
			fieldsByTable[table] = append(byTable, column)
			constructorsByTable[table][column] = constructor
			columnMetaByTable[table][column] = meta
		}
		features := map[string]interface{}{}
		featuresUncast, ok := schema["features"]
		if ok {
			features, ok = featuresUncast.(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid type for `features`")
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
			return fmt.Errorf("No primary key for table: %s", table)
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
			return fmt.Errorf("Unknown table: %s", name)
		}
		allFields := fieldsByTable[name]

		entries := map[string][]types.Entry{}
		for pk, fields := range encoded {
			row := make([]types.Entry, len(fieldsByTable[name]))
			entries[pk] = row
			fields[primary] = pk
			for _, column := range allFields {
				value := fields[column]
				fullName := fmt.Sprintf("%s.%s", name, column)
				index, ok := indexMap[fullName]
				if !ok {
					return fmt.Errorf("unknown column: %s", fullName)
				}
				constructor := constructorsByTable[name][column]

				typedVal, err := constructor(value, featuresByColumnByTable[name][column])
				if err != nil {
					return fmt.Errorf("failed to init %s.%s for %s: %s", name, column, pk, err)
				}
				row[index] = typedVal
			}
		}
		// TODO use a constructor and Inserts -- that way the able can map
		// columns by name
		table := types.NewTable(fieldsByTable[name], entries, primary, constructorsByTable[name], featuresByColumnByTable[name], columnMetaByTable[name])
		db.Tables[name] = table
	}
	osm.db = db
	return nil
}

func (osm *ObjectStoreMapper) dumpSnapshot(db *types.Database, dst io.Writer) error {
	encoded := storage.EncodedDatabase{
		schemataTableName: db.Schemata,
	}
	for name, table := range db.Tables {
		encoded[name] = osm.encodeTable(table)
	}
	return osm.store.Write(dst, encoded)
}

func (osm *ObjectStoreMapper) encodeTable(table *types.Table) storage.EncodedTable {
	encodedTable := storage.EncodedTable{}
	for pk := range table.Entries {
		encodedTable[pk] = osm.encodedRow(table, pk)
	}
	return encodedTable
}

func (osm *ObjectStoreMapper) encodedRow(table *types.Table, pk string) storage.EncodedEntry {
	// TODO inconsistent use of entry in types and storage
	encodedEntry := storage.EncodedEntry{}
	pkCol := table.Primary()
	row := table.Entries[pk]
	for i, entry := range row {
		if i != pkCol {
			encodedEntry[table.Columns[i]] = entry.Encoded()
		}
	}
	return encodedEntry
}

func (osm *ObjectStoreMapper) StoreEntries() error {
	entries := osm.getAndPurgeUpdates()
	if strings.HasSuffix(osm.path, ".json") {
		dst, err := os.OpenFile(osm.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer dst.Close()
		return osm.dumpSnapshot(osm.db, dst)
	} else if strings.HasSuffix(osm.path, ".jql") {
		return osm.storeAsDirectory(entries)
	} else {
		return fmt.Errorf("invalid path: %s", osm.path)
	}
}

func (osm *ObjectStoreMapper) storeAsDirectory(entries map[GlobalKey]string) error {
	for name, table := range osm.db.Tables {
		err := osm.storeTableInDirectory(entries, name, table)
		if err != nil {
			return err
		}
	}
	return nil
}

func (osm *ObjectStoreMapper) storeTableInDirectory(selected map[GlobalKey]string, name string, table *types.Table) error {
	strategy := getShardStrategy(table)

	if strategy.primaryShards == 0 && strategy.secondaryShards == 0 {
		encodedTable := storage.EncodedTable{}
		for pk := range selectEntries(selected, name, table) {
			encodedTable[pk] = osm.encodedRow(table, pk)
		}
		return osm.writeShard(
			filepath.Join(osm.path, fmt.Sprintf("%s.json", name)),
			encodedTable,
		)
	} else if strategy.primaryShards == 256 && strategy.secondaryShards == 0 {
		encodedTables := map[string]storage.EncodedTable{}
		for pk, entries := range selectEntries(selected, name, table) {
			key := ""
			if selected == nil {
				key = sanitizeKey(entries[table.IndexOfField(strategy.shardBy)].Format(""))
			} else {
				key = sanitizeKey(selected[GlobalKey{Table: name, PK: pk}])
			}
			hash := byteHex(byteHash([]byte(key)))
			if _, ok := encodedTables[hash]; !ok {
				encodedTables[hash] = storage.EncodedTable{}
			}
			encodedTables[hash][pk] = osm.encodedRow(table, pk)
		}
		for hash, encoded := range encodedTables {
			err := osm.writeShard(
				filepath.Join(osm.path, name, fmt.Sprintf("%s.json", hash)),
				encoded,
			)
			if err != nil {
				return err
			}
		}
	} else if strategy.primaryShards == 256 && strategy.secondaryShards == -1 {
		encodedTables := map[string](map[string]storage.EncodedTable){}
		for pk, entries := range selectEntries(selected, name, table) {
			key := ""
			if selected == nil {
				key = sanitizeKey(entries[table.IndexOfField(strategy.shardBy)].Format(""))
			} else {
				key = sanitizeKey(selected[GlobalKey{Table: name, PK: pk}])
			}
			hash := byteHex(byteHash([]byte(key)))
			if _, ok := encodedTables[hash]; !ok {
				encodedTables[hash] = map[string]storage.EncodedTable{}
			}
			if _, ok := encodedTables[hash][key]; !ok {
				encodedTables[hash][key] = storage.EncodedTable{}
			}
			encodedTables[hash][key][pk] = osm.encodedRow(table, pk)
		}
		for hash, tables := range encodedTables {
			for key, encoded := range tables {
				err := osm.writeShard(
					filepath.Join(osm.path, name, hash, fmt.Sprintf("%s.json", key)),
					encoded,
				)
				if err != nil {
					return err
				}
			}
		}
	} else {
		return fmt.Errorf("Unknown sharding strategy with %d primary shards and %d secondary shards", strategy.primaryShards, strategy.secondaryShards)
	}
	return nil
}

// writeShard upserts the provided entries into the shard. Any entries that are nil get deleted.
func (osm *ObjectStoreMapper) writeShard(path string, t storage.EncodedTable) error {
	reader, err := os.Open(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	shard, err := osm.store.ReadShard(reader)
	if err != nil {
		return err
	}
	for pk, row := range t {
		if len(row) == 0 {
			delete(shard, pk)
		} else {
			shard[pk] = row
		}
	}
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dst.Close()
	return osm.store.WriteShard(dst, shard)
}

func (osm *ObjectStoreMapper) GetSnapshot(db *types.Database) ([]byte, error) {
	var snapshot bytes.Buffer
	err := osm.dumpSnapshot(db, &snapshot)
	if err != nil {
		return nil, err
	}
	return snapshot.Bytes(), nil
}

func byteHash(b []byte) byte {
	hasher := fnv.New32a()
	hasher.Write(b)
	hashValue := hasher.Sum32()

	byte1 := byte(hashValue & 0xFF)
	byte2 := byte((hashValue >> 8) & 0xFF)
	byte3 := byte((hashValue >> 16) & 0xFF)
	byte4 := byte((hashValue >> 24) & 0xFF)

	return byte1 ^ byte2 ^ byte3 ^ byte4
}

func byteHex(b byte) string {
	return fmt.Sprintf("%02x", b)
}

func sanitizeKey(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}

type GlobalKey struct {
	Table    string
	PK       string
}

func selectEntries(entries map[GlobalKey]string, name string, table *types.Table) map[string][]types.Entry {
	if entries == nil {
		return table.Entries
	}
	selections := map[string][]types.Entry{}
	for gk := range entries {
		if gk.Table != name {
			continue
		}
		selections[gk.PK] = table.Entries[gk.PK]
	}
	return selections
}

type shardStrategy struct {
	shardBy         string
	primaryShards   int
	secondaryShards int
}

func getShardStrategy(table *types.Table) shardStrategy {
	strategy := shardStrategy{}

	// NOTE this will not error if a user has multiple shard
	// keys even though that's not supported
	for cname, meta := range table.ColumnMeta {
		if meta.PrimaryShards != 0 || meta.SecondaryShards != 0 {
			strategy.shardBy = cname
			strategy.primaryShards = meta.PrimaryShards
			strategy.secondaryShards = meta.SecondaryShards
		}
	}
	return strategy
}
