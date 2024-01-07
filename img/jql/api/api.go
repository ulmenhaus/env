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

type JQL_API interface {
	jqlpb.JQLClient
}

type LocalAPI struct {
	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	path string
}

func NewLocalAPI(mapper *osm.ObjectStoreMapper, db *types.Database, path string) (*LocalAPI, error) {
	return &LocalAPI{
		OSM: mapper,
		DB:  db,

		path: path,
	}, nil
}

func (s *LocalAPI) ListEntries(ctx context.Context, in *jqlpb.ListEntriesRequest, opts ...grpc.CallOption) (*jqlpb.ListEntriesResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *LocalAPI) WriteEntry(ctx context.Context, r *jqlpb.WriteEntryRequest, opts ...grpc.CallOption) (*jqlpb.WriteEntryResponse, error) {
	// NOTE the default behavior is an upsert with explicit fields to enforce inserting/updating
	// that are not implemented
	table, ok := s.DB.Tables[r.GetTable()]
	if !ok {
		return nil, fmt.Errorf("table does not exist")
	}
	table.InsertWithFields(r.GetPk(), r.GetFields())
	return &jqlpb.WriteEntryResponse{}, nil
}

func (s *LocalAPI) GetEntry(ctx context.Context, in *jqlpb.GetEntryRequest, opts ...grpc.CallOption) (*jqlpb.GetEntryResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *LocalAPI) DeleteEntry(ctx context.Context, in *jqlpb.DeleteEntryRequest, opts ...grpc.CallOption) (*jqlpb.DeleteEntryResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *LocalAPI) Persist(ctx context.Context, r *jqlpb.PersistRequest, opts ...grpc.CallOption) (*jqlpb.PersistResponse, error) {
	// TODO this prserves the existing interface used by all jql tools, but we should hide
	// all this logic behind the OSM
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return &jqlpb.PersistResponse{}, err
	}
	defer f.Close()
	return &jqlpb.PersistResponse{}, s.OSM.StoreEntries(s.DB)
}

// APIShim is a layer on top of the LocalAPI that provides gRPC handles for exposing the API as a daemon
type APIShim struct {
	*jqlpb.UnimplementedJQLServer
	api JQL_API
}

func NewAPIShim(api JQL_API) *APIShim {
	return &APIShim{
		api: api,
	}
}

func (s *APIShim) ListEntries(ctx context.Context, in *jqlpb.ListEntriesRequest) (*jqlpb.ListEntriesResponse, error) {
	return s.api.ListEntries(ctx, in)
}

func (s *APIShim) GetEntry(ctx context.Context, in *jqlpb.GetEntryRequest) (*jqlpb.GetEntryResponse, error) {
	return s.api.GetEntry(ctx, in)
}

func (s *APIShim) WriteEntry(ctx context.Context, in *jqlpb.WriteEntryRequest) (*jqlpb.WriteEntryResponse, error) {
	return s.api.WriteEntry(ctx, in)
}

func (s *APIShim) DeleteEntry(ctx context.Context, in *jqlpb.DeleteEntryRequest) (*jqlpb.DeleteEntryResponse, error) {
	return s.api.DeleteEntry(ctx, in)
}

func (s *APIShim) Persist(ctx context.Context, in *jqlpb.PersistRequest) (*jqlpb.PersistResponse, error) {
	return s.api.Persist(ctx, in)
}
