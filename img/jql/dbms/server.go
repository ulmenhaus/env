package dbms

import (
	"context"
	"fmt"
	"os"

	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

type DatabaseServer struct {
	*jqlpb.UnimplementedJQLServer

	OSM *osm.ObjectStoreMapper
	DB  *types.Database

	path string
}

func NewDatabaseServer(mapper *osm.ObjectStoreMapper, db *types.Database, path string) (*DatabaseServer, error) {
	return &DatabaseServer{
		OSM: mapper,
		DB:  db,

		path: path,
	}, nil
}

func (s *DatabaseServer) WriteEntry(ctx context.Context, r *jqlpb.WriteEntryRequest) (*jqlpb.WriteEntryResponse, error) {
	// NOTE the default behavior is an upsert with explicit fields to enforce inserting/updating
	// that are not implemented
	table, ok := s.DB.Tables[r.GetTable()]
	if !ok {
		return nil, fmt.Errorf("table does not exist")
	}
	table.InsertWithFields(r.GetPk(), r.GetFields())
	return &jqlpb.WriteEntryResponse{}, nil
}

func (s *DatabaseServer) Persist(ctx context.Context, r *jqlpb.PersistRequest) (*jqlpb.PersistResponse, error) {
	// TODO this prserves the existing interface used by all jql tools, but we should hide
	// all this logic behind the OSM
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return &jqlpb.PersistResponse{}, err
	}
	defer f.Close()
	return &jqlpb.PersistResponse{}, s.OSM.StoreEntries(s.DB)
}
