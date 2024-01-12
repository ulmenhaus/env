package api

import (
	"context"
	"errors"
	"fmt"
	"os"

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

func (s *LocalDBMS) ListRows(ctx context.Context, in *jqlpb.ListRowsRequest, opts ...grpc.CallOption) (*jqlpb.ListRowsResponse, error) {
	if len(in.Conditions) > 1 {
		return nil, errors.New("lisiting with multiple conditions is not yet implemented")
	}
	table, ok := s.OSM.GetDB().Tables[in.GetTable()]
	if !ok {
		return nil, fmt.Errorf("table does not exist: '%s'", in.GetTable())
	}
	resp, err := table.Query(types.QueryParams{
		OrderBy: in.GetOrderBy(),
		Dec:     in.GetDec(),
		Offset:  uint(in.GetOffset()),
		Limit:   uint(in.GetLimit()),
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
		Columns: columns,
		Rows:    rows,
		Total:   uint32(resp.Total),
	}, nil
}

func (s *LocalDBMS) WriteRow(ctx context.Context, in *jqlpb.WriteRowRequest, opts ...grpc.CallOption) (*jqlpb.WriteRowResponse, error) {
	// NOTE the default behavior is an upsert with explicit fields to enforce inserting/updating
	// that are not implemented
	table, ok := s.OSM.GetDB().Tables[in.GetTable()]
	if !ok {
		return nil, fmt.Errorf("table does not exist: '%s'", in.GetTable())
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
	table, ok := s.OSM.GetDB().Tables[in.GetTable()]
	if !ok {
		return nil, fmt.Errorf("table does not exist")
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
		Columns: columns,
		Row: &jqlpb.Row{
			Entries: entries,
		},
	}, nil
}

func (s *LocalDBMS) generateResponseColumns(table *types.Table) ([]*jqlpb.Column, error) {
	var columns []*jqlpb.Column
	for _, colname := range table.Columns {
		meta, ok := table.ColumnMeta[colname]
		if !ok {
			return nil, fmt.Errorf("could not find metadata for column: %s", colname)
		}
		columns = append(columns, &jqlpb.Column{
			Name:      colname,
			Type:      meta.Type,
			MaxLength: int32(meta.MaxLength),
		})
	}
	return columns, nil
}

func (s *LocalDBMS) DeleteRow(ctx context.Context, in *jqlpb.DeleteRowRequest, opts ...grpc.CallOption) (*jqlpb.DeleteRowResponse, error) {
	return nil, errors.New("not implemented")
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

func (s *DBMSShim) Persist(ctx context.Context, in *jqlpb.PersistRequest) (*jqlpb.PersistResponse, error) {
	return s.api.Persist(ctx, in)
}

func IndexOfField(columns []*jqlpb.Column, fieldName string) int {
	for i, col := range columns {
		if col.GetName() == fieldName {
			return i
		}
	}
	return -1
}
