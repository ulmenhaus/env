package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/img/jql/dbms"
	"github.com/ulmenhaus/env/img/jql/ui"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/grpc"
)

func main() {
	err := initJQL()
	if err != nil {
		panic(err)
	}
}

func initJQL() error {
	// TODO use a cli library
	dbPath := os.Args[1]
	tableName := os.Args[2]
	mapper, db, err := initDatabse(dbPath)
	if err != nil {
		return err
	}
	// placeholders for an eventual CLI args
	// TODO path should be hidden behind the OSM and never used directly by any of these components
	mode := "standalone"
	addr := "localhost:9999"
	switch mode {
	case "standalone":
		return initStandalone(dbPath, tableName, mapper, db)
	case "daemon":
		return initDaemon(dbPath, tableName, mapper, db, addr)
	case "client":
		return initClient(dbPath, tableName, mapper, db)
	default:
		return fmt.Errorf("Unknown init mode: %v", mode)
	}
}

func initDaemon(dbPath, tableName string, mapper *osm.ObjectStoreMapper, db *types.Database, addr string) error {
	server, err := dbms.NewDatabaseServer(mapper, db, dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database server: %v", err)
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	jqlpb.RegisterJQLServer(s, server)
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

func initClient(dbPath, tableName string, mapper *osm.ObjectStoreMapper, db *types.Database) error {
	return fmt.Errorf("Client mode not yet implemented")
}

func initStandalone(dbPath, tableName string, mapper *osm.ObjectStoreMapper, db *types.Database) error {
	mv, err := ui.NewMainView(dbPath, tableName, mapper, db)
	if err != nil {
		return err
	}
	if len(os.Args) > 3 {
		pk := os.Args[3]
		err = mv.GoToPrimaryKey(pk)
		if err != nil {
			return err
		}
	}
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return err
	}
	defer g.Close()
	g.InputEsc = true

	g.SetManagerFunc(mv.Layout)

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func initDatabse(path string) (*osm.ObjectStoreMapper, *types.Database, error) {
	var store storage.Store
	if strings.HasSuffix(path, ".json") {
		store = &storage.JSONStore{}
	} else {
		return nil, nil, fmt.Errorf("unknown file type")
	}
	mapper, err := osm.NewObjectStoreMapper(store)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	db, err := mapper.Load(f)
	if err != nil {
		return nil, nil, err
	}
	return mapper, db, err
}
