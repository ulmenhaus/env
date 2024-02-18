package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	flag "github.com/spf13/pflag"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/grpc"
)

const (
	ModeDaemon     = "daemon"
	ModeClient     = "client"
	ModeStandalone = "standalone"

	MaxPayloadSize = 50000000 // 50 Mb
)

type JQLConfig struct {
	Mode string

	Path  string
	Table string
	Addr  string
	VirtualGateway string

	PK string
}

func (c *JQLConfig) Validate() error {
	switch c.Mode {
	case ModeDaemon:
		if c.Path == "" {
			return fmt.Errorf("Path must be provided for daemon mode")
		}
		if c.Table != "" {
			return fmt.Errorf("Table cannot be provided for daemon mode")
		}
		if c.Addr == "" {
			return fmt.Errorf("Address must be provided for daemon mode")
		}
	case ModeClient:
		if c.Path != "" {
			return fmt.Errorf("Path cannot be provided for client mode")
		}
		if c.Table == "" {
			return fmt.Errorf("Table must be provided for client mode")
		}
		if c.Addr == "" {
			return fmt.Errorf("Address must be provided for client mode")
		}
	case ModeStandalone:
		if c.Path == "" {
			return fmt.Errorf("Path must be provided for standalone mode")
		}
		if c.Table == "" {
			return fmt.Errorf("Table must be provided for standalone mode")
		}
	default:
		return fmt.Errorf("Unknown mode")
	}
	return nil
}

func (c *JQLConfig) Register(f *flag.FlagSet) {
	f.StringVarP(&c.Mode, "mode", "m", "standalone", "Mode of operation")
	f.StringVarP(&c.Addr, "addr", "a", "localhost:9999", "Address (for remote connections)")
	f.StringVarP(&c.Path, "path", "p", "", "Path to the jql storage")
	f.StringVarP(&c.Table, "table", "t", "", "The table to start on")
	f.StringVarP(&c.PK, "pk", "", "", "The primary key to initially select")
	f.StringVarP(&c.VirtualGateway, "virtual-gateway", "", "", "The address where the virtual gateway runs")
}

func (c *JQLConfig) SwitchTool(tool, pk string) error {
	binary, err := exec.LookPath(tool)
	if err != nil {
		return err
	}

	args := []string{tool, "--mode", c.Mode, "--addr", c.Addr, "--path", c.Path, "--table", c.Table, "--pk", pk}

	env := os.Environ()

	err = syscall.Exec(binary, args, env)
	return err
}

func (c *JQLConfig) InitVirtualDBMS() (api.JQL_DBMS, error) {
	conn, err := grpc.Dial(
		c.VirtualGateway,
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxPayloadSize)),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(MaxPayloadSize)),
	)
	if err != nil {
		return nil, err
	}
	return jqlpb.NewJQLClient(conn), nil
}

func (c *JQLConfig) InitDBMS() (api.JQL_DBMS, error) {
	err := c.Validate()
	if err != nil {
		return nil, err
	}
	// As a convenience we reset the terminal when initializing the dbms
	// so that any previous attributes like highlights are gone
	clearTerminal()
	switch c.Mode {
	case ModeDaemon, ModeStandalone:
		mapper, err := osm.NewObjectStoreMapper(c.Path)
		if err != nil {
			return nil, err
		}
		err = mapper.Load()
		if err != nil {
			return nil, err
		}
		dbms, err := api.NewLocalDBMS(mapper, c.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database server: %v", err)
		}
		return dbms, err
	case ModeClient:
		conn, err := grpc.Dial(
			c.Addr,
			grpc.WithInsecure(),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxPayloadSize)),
			grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(MaxPayloadSize)),
		)
		if err != nil {
			return nil, err
		}
		// TODO return the closer so that it may be closed by the higher-level caller
		return api.NewRemoteDBMS(c.Addr, jqlpb.NewJQLClient(conn)), nil
	}
	return nil, fmt.Errorf("Unknown mode")
}

func clearTerminal() {
	fmt.Print("\033[0m") // Reset terminal attributes
	fmt.Print("\033[2J") // Clear the terminal screen
}
