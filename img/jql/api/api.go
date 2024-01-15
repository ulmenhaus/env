package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/grpc"
)

type JQL_DBMS interface {
	jqlpb.JQLClient
}

type LocalDBMS struct {
	OSM *osm.ObjectStoreMapper

	path string
}

func NewLocalDBMS(mapper *osm.ObjectStoreMapper, path string) (*LocalDBMS, error) {
	return &LocalDBMS{
		OSM: mapper,

		path: path,
	}, nil
}

// findTable takes in a user-provided table name and returns
// either that table if it's an exact match for a table, or
// the first table to match the provided prefix, or an error if no
// table matches
func (s *LocalDBMS) findTable(t string) (string, *types.Table, error) {
	table, ok := s.OSM.GetDB().Tables[t]
	if ok {
		return t, table, nil
	}
	for name, table := range s.OSM.GetDB().Tables {
		if strings.HasPrefix(name, t) {
			return name, table, nil
		}
	}
	return "", nil, fmt.Errorf("table does not exist: %s", t)
}

func (s *LocalDBMS) ListTables(ctx context.Context, in *jqlpb.ListTablesRequest, opts ...grpc.CallOption) (*jqlpb.ListTablesResponse, error) {
	var tables []*jqlpb.TableMeta

	for name, table := range s.OSM.GetDB().Tables {
		columns, err := s.generateResponseColumns(table)
		if err != nil {
			return nil, err
		}
		tables = append(tables, &jqlpb.TableMeta{
			Name:    name,
			Columns: columns,
		})
	}
	return &jqlpb.ListTablesResponse{
		Tables: tables,
	}, nil
}

func (s *LocalDBMS) ListRows(ctx context.Context, in *jqlpb.ListRowsRequest, opts ...grpc.CallOption) (*jqlpb.ListRowsResponse, error) {
	if len(in.Conditions) > 1 {
		return nil, errors.New("lisiting with multiple conditions is not yet implemented")
	}
	name, table, err := s.findTable(in.GetTable())
	if err != nil {
		return nil, err
	}
	var filters []types.Filter
	if len(in.Conditions) == 1 {
		for _, filter := range in.Conditions[0].Requires {
			filters = append(filters, &filterShim{
				filter: filter,
				colix:  table.IndexOfField(filter.GetColumn()),
			})
		}
	}
	resp, err := table.Query(types.QueryParams{
		OrderBy: in.GetOrderBy(),
		Dec:     in.GetDec(),
		Offset:  uint(in.GetOffset()),
		Limit:   uint(in.GetLimit()),
		// TODO now that we are filtering on the server-side with a fixed set of types
		// we can support more sophisticated indexing rather than just applying
		// filters in a linear fashion
		Filters: filters,
	})
	if err != nil {
		return nil, err
	}
	columns, err := s.generateResponseColumns(table)
	if err != nil {
		return nil, err
	}
	var rows []*jqlpb.Row
	for _, row := range resp.Entries {
		var entries []*jqlpb.Entry
		for _, entry := range row {
			entries = append(entries, &jqlpb.Entry{
				Formatted: entry.Format(""),
			})
		}
		rows = append(rows, &jqlpb.Row{
			Entries: entries,
		})
	}
	return &jqlpb.ListRowsResponse{
		Table:   name,
		Columns: columns,
		Rows:    rows,
		Total:   uint32(resp.Total),
		All:     uint32(len(table.Entries)),
	}, nil
}

func (s *LocalDBMS) WriteRow(ctx context.Context, in *jqlpb.WriteRowRequest, opts ...grpc.CallOption) (*jqlpb.WriteRowResponse, error) {
	// NOTE the default behavior is an upsert with explicit fields to enforce inserting/updating
	// that are not implemented
	_, table, err := s.findTable(in.GetTable())
	if err != nil {
		return nil, err
	}
	if in.GetUpdateOnly() {
		for key, value := range in.GetFields() {
			if err := table.Update(in.GetPk(), key, value); err != nil {
				return nil, err
			}
		}
	} else {
		table.InsertWithFields(in.GetPk(), in.GetFields())
	}
	return &jqlpb.WriteRowResponse{}, nil
}

func (s *LocalDBMS) GetRow(ctx context.Context, in *jqlpb.GetRowRequest, opts ...grpc.CallOption) (*jqlpb.GetRowResponse, error) {
	name, table, err := s.findTable(in.GetTable())
	if err != nil {
		return nil, err
	}
	row, ok := table.Entries[in.GetPk()]
	if !ok {
		return nil, fmt.Errorf("no such pk '%s' in table '%s'", in.GetPk(), in.GetTable())
	}
	var entries []*jqlpb.Entry
	for _, entry := range row {
		entries = append(entries, &jqlpb.Entry{
			Formatted: entry.Format(""),
		})
	}
	columns, err := s.generateResponseColumns(table)
	if err != nil {
		return nil, err
	}
	return &jqlpb.GetRowResponse{
		Table:   name,
		Columns: columns,
		Row: &jqlpb.Row{
			Entries: entries,
		},
	}, nil
}

func (s *LocalDBMS) generateResponseColumns(table *types.Table) ([]*jqlpb.Column, error) {
	var columns []*jqlpb.Column
	for i, colname := range table.Columns {
		meta, ok := table.ColumnMeta[colname]
		if !ok {
			return nil, fmt.Errorf("could not find metadata for column: %s", colname)
		}
		col := &jqlpb.Column{
			Name:         colname,
			Type:         meta.Type,
			MaxLength:    int32(meta.MaxLength),
			Primary:      table.Primary() == i,
			ForeignTable: meta.ForeignTable,
		}
		// TODO not a great interface. It'd be better to have the enum values as a field on the column metadata
		// Instead we pick a row if it exists and try to get the values from it
		for _, row := range table.Entries {
			enum, ok := row[i].(types.Enum)
			if ok {
				col.Values = enum.Values()
			}
			break
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (s *LocalDBMS) DeleteRow(ctx context.Context, in *jqlpb.DeleteRowRequest, opts ...grpc.CallOption) (*jqlpb.DeleteRowResponse, error) {
	_, table, err := s.findTable(in.GetTable())
	if err != nil {
		return nil, err
	}
	return &jqlpb.DeleteRowResponse{}, table.Delete(in.GetPk())
}

func (s *LocalDBMS) IncrementEntry(ctx context.Context, in *jqlpb.IncrementEntryRequest, opts ...grpc.CallOption) (*jqlpb.IncrementEntryResponse, error) {
	_, table, err := s.findTable(in.GetTable())
	if err != nil {
		return nil, err
	}
	row, ok := table.Entries[in.GetPk()]
	if !ok {
		return nil, fmt.Errorf("no such pk '%s' in table '%s'", in.GetPk(), in.GetTable())
	}
	colix := table.IndexOfField(in.GetColumn())
	if colix == -1 {
		return nil, fmt.Errorf("no such column '%s' in table '%s'", in.GetColumn(), in.GetTable())
	}
	entry := row[colix]
	// TODO leaky abstraction
	switch typed := entry.(type) {
	case types.ForeignKey:
		ftable := s.OSM.GetDB().Tables[typed.Table]
		// TODO not cache mapping
		fresp, err := ftable.Query(types.QueryParams{
			OrderBy: ftable.Columns[ftable.Primary()],
		})
		if err != nil {
			return nil, err
		}
		index := map[string]int{}
		for i, fentry := range fresp.Entries {
			index[fentry[ftable.Primary()].Format("")] = i
		}
		next := (index[entry.Format("")] + 1) % len(fresp.Entries)
		err = table.Update(in.GetPk(), in.GetColumn(), fresp.Entries[next][ftable.Primary()].Format(""))
		if err != nil {
			return nil, err
		}
	default:
		// TODO should use an Update so table can modify any necessary internals
		new, err := entry.Add(int(in.Amount))
		if err != nil {
			return nil, err
		}
		row[colix] = new
	}
	return &jqlpb.IncrementEntryResponse{}, nil
}

func (s *LocalDBMS) Persist(ctx context.Context, r *jqlpb.PersistRequest, opts ...grpc.CallOption) (*jqlpb.PersistResponse, error) {
	// TODO this prserves the existing interface used by all jql tools, but we should hide
	// all this logic behind the OSM
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return &jqlpb.PersistResponse{}, err
	}
	defer f.Close()
	return &jqlpb.PersistResponse{}, s.OSM.StoreEntries()
}

func (s *LocalDBMS) GetSnapshot(ctx context.Context, r *jqlpb.GetSnapshotRequest, opts ...grpc.CallOption) (*jqlpb.GetSnapshotResponse, error) {
	snapshot, err := s.OSM.GetSnapshot(s.OSM.GetDB())
	if err != nil {
		return nil, err
	}
	return &jqlpb.GetSnapshotResponse{
		Snapshot: snapshot,
	}, nil
}

func (s *LocalDBMS) LoadSnapshot(ctx context.Context, r *jqlpb.LoadSnapshotRequest, opts ...grpc.CallOption) (*jqlpb.LoadSnapshotResponse, error) {
	err := s.OSM.LoadSnapshot(bytes.NewBuffer(r.Snapshot))
	if err != nil {
		return nil, err
	}
	return &jqlpb.LoadSnapshotResponse{}, nil
}

// DBMSShim is a layer on top of the LocalDBMS that provides gRPC handles for exposing the DBMS as a daemon
type DBMSShim struct {
	*jqlpb.UnimplementedJQLServer
	api JQL_DBMS
}

func NewDBMSShim(api JQL_DBMS) *DBMSShim {
	return &DBMSShim{
		api: api,
	}
}

func (s *DBMSShim) ListTables(ctx context.Context, in *jqlpb.ListTablesRequest) (*jqlpb.ListTablesResponse, error) {
	return s.api.ListTables(ctx, in)
}

func (s *DBMSShim) ListRows(ctx context.Context, in *jqlpb.ListRowsRequest) (*jqlpb.ListRowsResponse, error) {
	return s.api.ListRows(ctx, in)
}

func (s *DBMSShim) GetRow(ctx context.Context, in *jqlpb.GetRowRequest) (*jqlpb.GetRowResponse, error) {
	return s.api.GetRow(ctx, in)
}

func (s *DBMSShim) WriteRow(ctx context.Context, in *jqlpb.WriteRowRequest) (*jqlpb.WriteRowResponse, error) {
	return s.api.WriteRow(ctx, in)
}

func (s *DBMSShim) DeleteRow(ctx context.Context, in *jqlpb.DeleteRowRequest) (*jqlpb.DeleteRowResponse, error) {
	return s.api.DeleteRow(ctx, in)
}

func (s *DBMSShim) IncrementEntry(ctx context.Context, in *jqlpb.IncrementEntryRequest) (*jqlpb.IncrementEntryResponse, error) {
	return s.api.IncrementEntry(ctx, in)
}

func (s *DBMSShim) Persist(ctx context.Context, in *jqlpb.PersistRequest) (*jqlpb.PersistResponse, error) {
	return s.api.Persist(ctx, in)
}

func (s *DBMSShim) GetSnapshot(ctx context.Context, in *jqlpb.GetSnapshotRequest) (*jqlpb.GetSnapshotResponse, error) {
	return s.api.GetSnapshot(ctx, in)
}

func (s *DBMSShim) LoadSnapshot(ctx context.Context, in *jqlpb.LoadSnapshotRequest) (*jqlpb.LoadSnapshotResponse, error) {
	return s.api.LoadSnapshot(ctx, in)
}

func IndexOfField(columns []*jqlpb.Column, fieldName string) int {
	for i, col := range columns {
		if col.GetName() == fieldName {
			return i
		}
	}
	return -1
}

func GetPrimary(columns []*jqlpb.Column) int {
	for i, col := range columns {
		if col.GetPrimary() {
			return i
		}
	}
	return -1
}

// HasForeign returns the index of the column that is a foriegn key to the
// provided table or -1 if there is no such column
func HasForeign(columns []*jqlpb.Column, table string) int {
	for i, column := range columns {
		if column.ForeignTable == table {
			return i
		}
	}
	return -1
}
