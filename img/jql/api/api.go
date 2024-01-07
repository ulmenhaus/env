package api

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ulmenhaus/env/img/jql/osm"
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

func (s *LocalDBMS) ListEntries(ctx context.Context, in *jqlpb.ListEntriesRequest, opts ...grpc.CallOption) (*jqlpb.ListEntriesResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *LocalDBMS) WriteEntry(ctx context.Context, r *jqlpb.WriteEntryRequest, opts ...grpc.CallOption) (*jqlpb.WriteEntryResponse, error) {
	// NOTE the default behavior is an upsert with explicit fields to enforce inserting/updating
	// that are not implemented
	table, ok := s.OSM.GetDB().Tables[r.GetTable()]
	if !ok {
		return nil, fmt.Errorf("table does not exist")
	}
	table.InsertWithFields(r.GetPk(), r.GetFields())
	return &jqlpb.WriteEntryResponse{}, nil
}

func (s *LocalDBMS) GetEntry(ctx context.Context, in *jqlpb.GetEntryRequest, opts ...grpc.CallOption) (*jqlpb.GetEntryResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *LocalDBMS) DeleteEntry(ctx context.Context, in *jqlpb.DeleteEntryRequest, opts ...grpc.CallOption) (*jqlpb.DeleteEntryResponse, error) {
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

func (s *DBMSShim) ListEntries(ctx context.Context, in *jqlpb.ListEntriesRequest) (*jqlpb.ListEntriesResponse, error) {
	return s.api.ListEntries(ctx, in)
}

func (s *DBMSShim) GetEntry(ctx context.Context, in *jqlpb.GetEntryRequest) (*jqlpb.GetEntryResponse, error) {
	return s.api.GetEntry(ctx, in)
}

func (s *DBMSShim) WriteEntry(ctx context.Context, in *jqlpb.WriteEntryRequest) (*jqlpb.WriteEntryResponse, error) {
	return s.api.WriteEntry(ctx, in)
}

func (s *DBMSShim) DeleteEntry(ctx context.Context, in *jqlpb.DeleteEntryRequest) (*jqlpb.DeleteEntryResponse, error) {
	return s.api.DeleteEntry(ctx, in)
}

func (s *DBMSShim) Persist(ctx context.Context, in *jqlpb.PersistRequest) (*jqlpb.PersistResponse, error) {
	return s.api.Persist(ctx, in)
}
