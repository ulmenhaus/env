package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	OSM  *osm.ObjectStoreMapper
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
			shim := newFilterShim(filter, table)
			filters = append(filters, shim)
		}
	}
	groupings, additionalFilters, err := s.calculateGroupings(in, table, filters)
	if err != nil {
		return nil, err
	}
	filters = append(filters, additionalFilters...)
	orderBy := in.GetOrderBy()
	if orderBy == "" {
		orderBy = table.Columns[table.Primary()]
	}
	resp, err := table.Query(types.QueryParams{
		OrderBy: orderBy,
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
		Table:     name,
		Columns:   columns,
		Rows:      rows,
		Total:     uint32(resp.Total),
		All:       uint32(len(table.Entries)),
		Groupings: groupings,
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
		s.OSM.RowUpdating(in.GetTable(), in.GetPk())
		// Take two passes here, one for updating non-pk fields
		// and one for updating the pk. If the pk is updated before other
		// fields, subsequent updates can't work
		for key, value := range in.GetFields() {
			if table.Primary() == table.IndexOfField(key) {
				continue
			}
			if err := table.Update(in.GetPk(), key, value); err != nil {
				return nil, err
			}
		}
		for key, value := range in.GetFields() {
			if table.Primary() != table.IndexOfField(key) {
				continue
			}
			if err := table.Update(in.GetPk(), key, value); err != nil {
				return nil, err
			}
			s.OSM.RowUpdating(in.GetTable(), value)
		}
	} else {
		table.InsertWithFields(in.GetPk(), in.GetFields())
		s.OSM.RowUpdating(in.GetTable(), in.GetPk())
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
	s.OSM.RowUpdating(in.GetTable(), in.GetPk())
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
	s.OSM.RowUpdating(in.GetTable(), in.GetPk())
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
	var err error
	// We mark all keys as updated both before and after loading the snapshot. This is because any keys which no longer
	// exist after the load should be marked for purging and any new keys should be marked for writing.
	if r.Snapshot == nil {
		err = s.OSM.Load()
	} else {
		err = s.OSM.LoadSnapshot(bytes.NewBuffer(r.Snapshot))
	}
	if err != nil {
		return nil, err
	}
	return &jqlpb.LoadSnapshotResponse{}, nil
}

func (s *LocalDBMS) calculateGroupings(in *jqlpb.ListRowsRequest, table *types.Table, filters []types.Filter) ([]*jqlpb.Grouping, []types.Filter, error) {
	if in.GroupBy == nil {
		return nil, nil, nil
	}
	resp, err := table.Query(types.QueryParams{
		Filters: filters,
	})
	if err != nil {
		return nil, nil, err
	}

	rows := resp.Entries
	groupings := []*jqlpb.Grouping{}
	additionalFilters := []types.Filter{}
	for _, requestedGrouping := range in.GroupBy.Groupings {
		values := map[string]int64{}
		filteredRows := [][]types.Entry{}
		for _, row := range rows {
			value := row[table.IndexOfField(requestedGrouping.Field)].Format("")
			values[value] += 1
			if value == requestedGrouping.Selected {
				filteredRows = append(filteredRows, row)
			}
		}
		groupings = append(groupings, &jqlpb.Grouping{
			Field:    requestedGrouping.Field,
			Values:   values,
			Selected: requestedGrouping.Selected,
		})
		additionalFilters = append(additionalFilters, newFilterShim(&jqlpb.Filter{
			Column: requestedGrouping.Field,
			Match:  &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: requestedGrouping.Selected}},
		}, table))
		rows = filteredRows
	}
	return groupings, additionalFilters, nil
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

// GetForeign returns the index of the columns that are foriegn keys to the
// provided table
func GetForeign(columns []*jqlpb.Column, table string) []int {
	var ret []int
	for i, column := range columns {
		if column.ForeignTable == table {
			ret = append(ret, i)
		}
	}
	return ret
}

func GetTables(ctx context.Context, dbms JQL_DBMS) (map[string]*jqlpb.TableMeta, error) {
	tablesList, err := dbms.ListTables(ctx, &jqlpb.ListTablesRequest{})
	if err != nil {
		return nil, err
	}
	tables := map[string]*jqlpb.TableMeta{}
	for _, table := range tablesList.Tables {
		tables[table.Name] = table
	}
	return tables, nil
}

// Router is a layer on top of the LocalDBMS that provides gRPC handles for exposing the DBMS as a daemon
type Router struct {
	*jqlpb.UnimplementedJQLServer
	api            jqlpb.JQLServer
	virtualGateway jqlpb.JQLServer
}

func NewRouter(api jqlpb.JQLServer, virtualGateway jqlpb.JQLServer) *Router {
	return &Router{
		api:            api,
		virtualGateway: virtualGateway,
	}
}

func (s *Router) ListTables(ctx context.Context, in *jqlpb.ListTablesRequest) (*jqlpb.ListTablesResponse, error) {
	return s.api.ListTables(ctx, in)
}

func (s *Router) ListRows(ctx context.Context, in *jqlpb.ListRowsRequest) (*jqlpb.ListRowsResponse, error) {
	if IsVirtualTable(in.Table) {
		return s.virtualGateway.ListRows(ctx, in)
	}
	return s.api.ListRows(ctx, in)
}

func (s *Router) GetRow(ctx context.Context, in *jqlpb.GetRowRequest) (*jqlpb.GetRowResponse, error) {
	if IsVirtualTable(in.Table) {
		return s.virtualGateway.GetRow(ctx, in)
	}
	return s.api.GetRow(ctx, in)
}

func (s *Router) WriteRow(ctx context.Context, in *jqlpb.WriteRowRequest) (*jqlpb.WriteRowResponse, error) {
	if IsVirtualTable(in.Table) {
		return s.virtualGateway.WriteRow(ctx, in)
	}
	return s.api.WriteRow(ctx, in)
}

func (s *Router) DeleteRow(ctx context.Context, in *jqlpb.DeleteRowRequest) (*jqlpb.DeleteRowResponse, error) {
	if IsVirtualTable(in.Table) {
		return s.virtualGateway.DeleteRow(ctx, in)
	}
	return s.api.DeleteRow(ctx, in)
}

func (s *Router) IncrementEntry(ctx context.Context, in *jqlpb.IncrementEntryRequest) (*jqlpb.IncrementEntryResponse, error) {
	if IsVirtualTable(in.Table) {
		return s.virtualGateway.IncrementEntry(ctx, in)
	}
	return s.api.IncrementEntry(ctx, in)
}

func (s *Router) Persist(ctx context.Context, in *jqlpb.PersistRequest) (*jqlpb.PersistResponse, error) {
	return s.api.Persist(ctx, in)
}

func (s *Router) GetSnapshot(ctx context.Context, in *jqlpb.GetSnapshotRequest) (*jqlpb.GetSnapshotResponse, error) {
	return s.api.GetSnapshot(ctx, in)
}

func (s *Router) LoadSnapshot(ctx context.Context, in *jqlpb.LoadSnapshotRequest) (*jqlpb.LoadSnapshotResponse, error) {
	return s.api.LoadSnapshot(ctx, in)
}

func IsVirtualTable(name string) bool {
	return strings.HasPrefix(name, "vt.")
}

func ConstructPolyForeign(table, pk string) string {
	return fmt.Sprintf("%s %s", table, pk)
}

func ParsePolyforeign(entry *jqlpb.Entry) (string, []string) {
	formatted := entry.Formatted

	parts := strings.SplitN(formatted, " ", 2)
	if len(parts) < 2 {
		return "", []string{}
	}
	return parts[0], []string{parts[1]}
}

func ParseLink(entry *jqlpb.Entry) (string, []string) {
	formatted := entry.Link

	parts := strings.SplitN(formatted, " ", 2)
	if len(parts) < 2 {
		return "", []string{}
	}
	return parts[0], []string{parts[1]}
}

func GetDisplayValue(entry *jqlpb.Entry) string {
	if entry.GetDisplayValue() != "" {
		return entry.GetDisplayValue()
	}
	return entry.GetFormatted()
}

type RemoteDBMS struct {
	jqlpb.JQLClient
	Address string
}

func NewRemoteDBMS(addr string, client jqlpb.JQLClient) *RemoteDBMS {
	return &RemoteDBMS{
		JQLClient: client,
		Address:   addr,
	}
}

func IsNotExistError(err error) bool {
	// TODO we can probably provide some richer error codes in the response
	// to be able to determine this
	return strings.Contains(err.Error(), "no such pk")
}
