package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-celestia/flags"
	"github.com/ethereum-optimism/optimism/op-celestia/indexer/rpc"
	"github.com/ethereum-optimism/optimism/op-celestia/indexer/store"
	"github.com/ethereum-optimism/optimism/op-celestia/metrics"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	"github.com/ethereum-optimism/optimism/op-service/client"
	"github.com/ethereum-optimism/optimism/op-service/dial"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/httputil"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/oppprof"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

var ErrAlreadyStopped = errors.New("already stopped")

// IndexerService represents the complete indexer service
type IndexerService struct {
	Log     log.Logger
	Metrics metrics.Metricer

	IndexerConfig

	L1Client       *combinedL1Client
	L2Client       *sources.L2Client
	OpNodeClient   dial.RollupClientInterface // optional
	CelestiaClient *celestia.DAClient
	Store          store.Store

	driver *IndexerDriver

	Version string

	pprofService *oppprof.Service
	metricsSrv   *httputil.HTTPServer
	rpcServer    *oprpc.Server

	stopped atomic.Bool
}

// IndexerServiceFromCLIConfig creates a new IndexerService from a CLIConfig
func IndexerServiceFromCLIConfig(ctx context.Context, version string, cfg *CLIConfig, log log.Logger) (*IndexerService, error) {
	var is IndexerService
	if err := is.initFromCLIConfig(ctx, version, cfg, log); err != nil {
		return nil, errors.Join(err, is.Stop(ctx))
	}
	return &is, nil
}

func (is *IndexerService) initFromCLIConfig(ctx context.Context, version string, cfg *CLIConfig, log log.Logger) error {
	is.Version = version
	is.Log = log

	is.initMetrics(cfg)
	is.initIndexerConfig(cfg)

	if err := is.initClients(ctx, cfg); err != nil {
		return err
	}
	if err := is.initStore(cfg); err != nil {
		return err
	}
	if err := is.initMetricsServer(cfg); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	if err := is.initPProf(cfg); err != nil {
		return fmt.Errorf("failed to init profiling: %w", err)
	}
	if err := is.initDriver(); err != nil {
		return fmt.Errorf("failed to init driver: %w", err)
	}
	if err := is.initRPCServer(cfg); err != nil {
		return fmt.Errorf("failed to start RPC server: %w", err)
	}

	is.Metrics.RecordInfo(is.Version)
	is.Metrics.RecordUp()
	return nil
}

func (is *IndexerService) initMetrics(cfg *CLIConfig) {
	if cfg.MetricsConfig.Enabled {
		procName := "default"
		is.Metrics = metrics.NewMetrics(procName)
	} else {
		is.Metrics = metrics.NoopMetrics
	}
}

func (is *IndexerService) initIndexerConfig(cfg *CLIConfig) {
	is.IndexerConfig = IndexerConfig{
		StartL1Block:      cfg.StartL1Block,
		BatchInboxAddress: common.HexToAddress(cfg.BatchInboxAddress),
		L1EthRpc:          cfg.L1EthRpc,
		L1BeaconRpc:       cfg.L1BeaconRpc,
		OpNodeRpc:         cfg.OpNodeRpc,
		PollInterval:      cfg.PollInterval,
		NetworkTimeout:    cfg.NetworkTimeout,
		CelestiaConfig:    cfg.CelestiaConfig,
		L2BlockTime:       cfg.L2BlockTime,
		L2GenesisTime:     cfg.L2GenesisTime,
		ChainID:           cfg.L2ChainID,
	}
}

func (is *IndexerService) initClients(ctx context.Context, cfg *CLIConfig) error {
	// Initialize L1 execution client
	elClient, err := dial.DialEthClientWithTimeout(ctx, dial.DefaultDialTimeout, is.Log, cfg.L1EthRpc)
	if err != nil {
		return fmt.Errorf("failed to dial L1 RPC: %w", err)
	}

	// Initialize optional L1 beacon client (for EIP-4844)
	var beaconClient *sources.L1BeaconClient
	if cfg.L1BeaconRpc != "" {
		httpCl := client.NewBasicHTTPClient(cfg.L1BeaconRpc, is.Log)
		beaconHTTP := sources.NewBeaconHTTPClient(httpCl)
		beaconCfg := sources.L1BeaconClientConfig{
			FetchAllSidecars: false, // TODO: https://ethereum.github.io/beacon-APIs/#/Beacon/getBlobSidecars is depricated, so we don't want this?
		}
		beaconClient = sources.NewL1BeaconClient(beaconHTTP, beaconCfg)
		is.Log.Info("Initialized L1 beacon client", "url", cfg.L1BeaconRpc)
	} else {
		is.Log.Warn("No L1 beacon URL configured; EIP-4844 blob DA batches will not be indexed")
	}

	// Combine EL + CL into a single L1Client implementation
	is.L1Client = &combinedL1Client{
		el: elClient,
		cl: beaconClient,
	}

	// Initialize op-node client
	opNodeClient, err := dial.DialRollupClientWithTimeout(ctx, is.Log, cfg.OpNodeRpc)
	if err != nil {
		return fmt.Errorf("Failed to dial op-node RPC, verification will be disabled: %w", err)
	}

	rollupConfig, err := opNodeClient.RollupConfig(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get rollup config: %w", err)
	}

	// Initialize L2 client
	l2Client, err := dial.DialEthClientWithTimeout(ctx, dial.DefaultDialTimeout, is.Log, cfg.L2EthRpc)
	if err != nil {
		return fmt.Errorf("failed to dial L2 RPC: %w", err)
	}
	l2RPC := client.NewBaseRPCClient(l2Client.Client())
	l2RpcClient, err := sources.NewL2Client(l2RPC, is.Log, nil, sources.L2ClientDefaultConfig(rollupConfig, false))
	if err != nil {
		return fmt.Errorf("failed to create L2 eth client")
	}
	is.L2Client = l2RpcClient

	// Initialize Celestia client
	celestiaClient, err := celestia.NewDAClient(cfg.CelestiaConfig.CelestiaConfig())
	if err != nil {
		return fmt.Errorf("failed to create Celestia client: %w", err)
	}
	is.CelestiaClient = celestiaClient

	return nil
}

func (is *IndexerService) initStore(cfg *CLIConfig) error {
	if cfg.DbPath == "" {
		is.Store = store.NewMemoryStore()
		return nil
	}
	sqliteStore, err := store.NewSqliteStore(cfg.DbPath)
	if err != nil {
		return fmt.Errorf("failed to create sqlite store: %w", err)
	}
	is.Store = sqliteStore
	return nil
}

func (is *IndexerService) initPProf(cfg *CLIConfig) error {
	is.pprofService = oppprof.New(
		cfg.PprofConfig.ListenEnabled,
		cfg.PprofConfig.ListenAddr,
		cfg.PprofConfig.ListenPort,
		cfg.PprofConfig.ProfileType,
		cfg.PprofConfig.ProfileDir,
		cfg.PprofConfig.ProfileFilename,
	)

	if err := is.pprofService.Start(); err != nil {
		return fmt.Errorf("failed to start pprof service: %w", err)
	}

	return nil
}

func (is *IndexerService) initMetricsServer(cfg *CLIConfig) error {
	if !cfg.MetricsConfig.Enabled {
		is.Log.Info("Metrics disabled")
		return nil
	}
	m, ok := is.Metrics.(opmetrics.RegistryMetricer)
	if !ok {
		return fmt.Errorf("metrics were enabled, but metricer %T does not expose registry for metrics-server", is.Metrics)
	}
	is.Log.Debug("Starting metrics server", "addr", cfg.MetricsConfig.ListenAddr, "port", cfg.MetricsConfig.ListenPort)
	metricsSrv, err := opmetrics.StartServer(m.Registry(), cfg.MetricsConfig.ListenAddr, cfg.MetricsConfig.ListenPort)
	if err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	is.Log.Info("Started metrics server", "addr", metricsSrv.Addr())
	is.metricsSrv = metricsSrv
	return nil
}

func (is *IndexerService) initDriver() error {
	var opNodeClient OpNodeClient
	if is.OpNodeClient != nil {
		opNodeClient = &opNodeWrapper{is.OpNodeClient}
	}

	driver := NewIndexerDriver(DriverSetup{
		Log:            is.Log,
		Metr:           is.Metrics,
		Cfg:            is.IndexerConfig,
		L1Client:       is.L1Client,
		L2Client:       is.L2Client,
		OpNodeClient:   opNodeClient,
		CelestiaClient: is.CelestiaClient,
		Store:          is.Store,
	})

	is.driver = driver
	return nil
}

func (is *IndexerService) initRPCServer(cfg *CLIConfig) error {
	server := oprpc.NewServer(
		cfg.RPCConfig.ListenAddr,
		cfg.RPCConfig.ListenPort,
		is.Version,
		oprpc.WithLogger(is.Log),
		oprpc.WithRPCRecorder(is.Metrics.NewRecorder("main")),
	)

	if cfg.RPCConfig.EnableAdmin {
		// Create an adapter that converts between indexer and RPC types
		adapter := &driverAdapter{driver: is.driver}
		indexerAPI := rpc.NewIndexerAPI(adapter, is.Log)
		server.AddAPI(rpc.GetAPI(indexerAPI))
		is.Log.Info("Admin RPC enabled")
	}

	is.Log.Info("Starting JSON-RPC server")
	if err := server.Start(); err != nil {
		return fmt.Errorf("unable to start RPC server: %w", err)
	}
	is.rpcServer = server
	return nil
}

// Start runs once upon start of the indexer lifecycle
func (is *IndexerService) Start(_ context.Context) error {
	is.Log.Info("Starting Celestia Indexer Service")
	return is.driver.Start()
}

// Stopped returns if the service is stopped
func (is *IndexerService) Stopped() bool {
	return is.stopped.Load()
}

// Kill forcefully stops the service
func (is *IndexerService) Kill() error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return is.Stop(ctx)
}

// Stop gracefully stops the indexer service
func (is *IndexerService) Stop(ctx context.Context) error {
	if is.stopped.Load() {
		return ErrAlreadyStopped
	}
	is.Log.Info("Stopping Celestia Indexer Service")

	var result error

	if is.driver != nil {
		if err := is.driver.Stop(); err != nil {
			result = errors.Join(result, fmt.Errorf("failed to stop driver: %w", err))
		}
	}

	if is.rpcServer != nil {
		if err := is.rpcServer.Stop(); err != nil {
			result = errors.Join(result, fmt.Errorf("failed to stop RPC server: %w", err))
		}
	}

	if is.pprofService != nil {
		if err := is.pprofService.Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("failed to stop PProf server: %w", err))
		}
	}

	if is.metricsSrv != nil {
		if err := is.metricsSrv.Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("failed to stop metrics server: %w", err))
		}
	}

	if is.L1Client != nil {
		is.L1Client.Close()
	}

	if is.OpNodeClient != nil {
		is.OpNodeClient.Close()
	}

	if result == nil {
		is.stopped.Store(true)
		is.Log.Info("Celestia Indexer Service stopped")
	}

	return result
}

// opNodeWrapper wraps the dial.RollupClientInterface to match our OpNodeClient interface
type opNodeWrapper struct {
	dial.RollupClientInterface
}

// combinedL1Client implements L1Client by wrapping EL and (optional) CL clients.
type combinedL1Client struct {
	el *ethclient.Client
	cl *sources.L1BeaconClient
}

func (c *combinedL1Client) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return c.el.BlockByNumber(ctx, number)
}

func (c *combinedL1Client) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return c.el.HeaderByNumber(ctx, number)
}

func (c *combinedL1Client) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return c.el.FilterLogs(ctx, query)
}

func (c *combinedL1Client) GetBlobs(ctx context.Context, ref eth.L1BlockRef, hashes []eth.IndexedBlobHash) ([]*eth.Blob, error) {
	if c.cl == nil {
		return nil, fmt.Errorf("blob DA not configured")
	}
	return c.cl.GetBlobs(ctx, ref, hashes)
}

func (c *combinedL1Client) Close() {
	if c.el != nil {
		c.el.Close()
	}
}

func (w *opNodeWrapper) OutputAtBlock(ctx context.Context, blockNum uint64) (*eth.OutputResponse, error) {
	return w.RollupClientInterface.OutputAtBlock(ctx, blockNum)
}

// driverAdapter adapts the IndexerDriver to the RPC interface by converting types
type driverAdapter struct {
	driver *IndexerDriver
}

func (da *driverAdapter) GetDALocation(l2BlockNum uint64) (store.DALocation, error) {
	return da.driver.GetDALocation(l2BlockNum)
}

func (da *driverAdapter) GetStatus() (lastIndexedBlock uint64, indexedBlocks int, running bool, l2Start uint64, l2End uint64, err error) {
	return da.driver.GetStatus()
}

var _ cliapp.Lifecycle = (*IndexerService)(nil)

// HTTPEndpoint returns the HTTP endpoint of the RPC server
func (is *IndexerService) HTTPEndpoint() string {
	if is.rpcServer == nil {
		return ""
	}
	return "http://" + is.rpcServer.Endpoint()
}

// Main is the entrypoint into the Celestia Indexer
func Main(version string) cliapp.LifecycleAction {
	return func(cliCtx *cli.Context, _ context.CancelCauseFunc) (cliapp.Lifecycle, error) {
		if err := flags.CheckRequired(cliCtx); err != nil {
			return nil, err
		}
		cfg := NewConfig(cliCtx)
		if err := cfg.Check(); err != nil {
			return nil, fmt.Errorf("invalid CLI flags: %w", err)
		}

		l := oplog.NewLogger(oplog.AppOut(cliCtx), cfg.LogConfig)
		oplog.SetGlobalLogHandler(l.Handler())
		opservice.ValidateEnvVars(flags.EnvVarPrefix, flags.Flags, l)

		l.Info("Initializing Celestia Indexer")
		return IndexerServiceFromCLIConfig(cliCtx.Context, version, cfg, l)
	}
}
