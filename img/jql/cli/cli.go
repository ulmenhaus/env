package cli

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	flag "github.com/spf13/pflag"
	"github.com/ulmenhaus/env/img/jql/api"
	"github.com/ulmenhaus/env/img/jql/osm"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
)

const (
	ModeDaemon     = "daemon"
	ModeClient     = "client"
	ModeStandalone = "standalone"

	MaxPayloadSize = 100000000 // 100 Mb
)

type Filter struct {
	Key   string
	Value string
}

type JQLConfig struct {
	Mode string

	Path           string
	Table          string
	Addr           string
	VirtualGateway string
	ListenUnix     string

	PK       string
	SelectPK string
	Query    string

	TLSCert string
	TLSKey  string
	TLSCA   string

	filters []string
}

func (c *JQLConfig) queryTable() string {
	if c.Query == "" {
		return ""
	}
	req, err := c.GetQuery()
	if err != nil {
		return ""
	}
	return req.Table
}

func (c *JQLConfig) Validate() error {
	if c.Query != "" && (c.Table != "" || len(c.filters) > 0) {
		return fmt.Errorf("--query cannot be used with --table or --filter")
	}
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
		if c.Table == "" && c.queryTable() == "" {
			return fmt.Errorf("Table must be provided for client mode")
		}
		if c.Addr == "" {
			return fmt.Errorf("Address must be provided for client mode")
		}
	case ModeStandalone:
		if c.Path == "" {
			return fmt.Errorf("Path must be provided for standalone mode")
		}
		if c.Table == "" && c.queryTable() == "" {
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
	f.StringVarP(&c.SelectPK, "select", "", "", "Place the cursor on the row with this primary key after initialization")
	f.StringVarP(&c.VirtualGateway, "virtual-gateway", "", "", "The address where the virtual gateway runs")
	f.StringVarP(&c.ListenUnix, "listen-unix", "", "", "Additional Unix socket path for the daemon to listen on")
	f.StringArrayVarP(&c.filters, "filter", "", []string{}, "Add initial filters to the table")
	f.StringVarP(&c.Query, "query", "", "", "Base64-encoded ListRowsRequest as the initial query (mutually exclusive with --table and --filter)")
	f.StringVarP(&c.TLSCert, "tls-cert", "", "", "Path to TLS certificate file")
	f.StringVarP(&c.TLSKey, "tls-key", "", "", "Path to TLS key file")
	f.StringVarP(&c.TLSCA, "tls-ca", "", "", "Path to TLS CA certificate file")
}

func (c *JQLConfig) SwitchTool(tool, pk string, filters ...Filter) error {
	binary, err := exec.LookPath(tool)
	if err != nil {
		return err
	}

	args := []string{tool, "--mode", c.Mode, "--addr", c.Addr, "--path", c.Path, "--table", c.Table, "--pk", pk}
	if c.TLSCert != "" {
		args = append(args, "--tls-cert", c.TLSCert)
	}
	if c.TLSKey != "" {
		args = append(args, "--tls-key", c.TLSKey)
	}
	if c.TLSCA != "" {
		args = append(args, "--tls-ca", c.TLSCA)
	}
	if c.ListenUnix != "" {
		args = append(args, "--listen-unix", c.ListenUnix)
	}
	if c.SelectPK != "" {
		args = append(args, "--select", c.SelectPK)
	}
	if c.Query != "" {
		args = append(args, "--query", c.Query)
	}
	for _, filter := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", filter.Key, filter.Value))
	}

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
		dialOpts := []grpc.DialOption{
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxPayloadSize)),
			grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(MaxPayloadSize)),
		}
		if c.TLSCert != "" {
			creds, err := c.clientCredentials()
			if err != nil {
				return nil, err
			}
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}
		conn, err := grpc.Dial(c.Addr, dialOpts...)
		if err != nil {
			return nil, err
		}
		// TODO return the closer so that it may be closed by the higher-level caller
		remote := api.NewRemoteDBMS(c.Addr, jqlpb.NewJQLClient(conn))
		remote.TLSCert = c.TLSCert
		remote.TLSKey = c.TLSKey
		remote.TLSCA = c.TLSCA
		return remote, nil
	}
	return nil, fmt.Errorf("Unknown mode")
}

// ServerCredentials returns a gRPC server option enforcing mTLS when TLS fields
// are set: clients must present a certificate signed by the configured CA.
// Returns nil, nil when TLS is not configured.
func (c *JQLConfig) ServerCredentials() (grpc.ServerOption, error) {
	if c.TLSCert == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(c.TLSCert, c.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load server cert/key: %w", err)
	}
	caCert, err := os.ReadFile(c.TLSCA)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	return grpc.Creds(credentials.NewTLS(tlsCfg)), nil
}

func (c *JQLConfig) clientCredentials() (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(c.TLSCert, c.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert/key: %w", err)
	}
	caCert, err := os.ReadFile(c.TLSCA)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
	}
	return credentials.NewTLS(tlsCfg), nil
}

func (c *JQLConfig) GetQuery() (*jqlpb.ListRowsRequest, error) {
	data, err := base64.StdEncoding.DecodeString(c.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to decode --query: %w", err)
	}
	req := &jqlpb.ListRowsRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal --query: %w", err)
	}
	return req, nil
}

func (c *JQLConfig) GetFilters() []Filter {
	filters := []Filter{}
	for _, s := range c.filters {
		if !strings.Contains(s, "=") {
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		filters = append(filters, Filter{
			Key:   parts[0],
			Value: parts[1],
		})
	}
	return filters
}

func clearTerminal() {
	fmt.Print("\033[0m") // Reset terminal attributes
	fmt.Print("\033[2J") // Clear the terminal screen
}
