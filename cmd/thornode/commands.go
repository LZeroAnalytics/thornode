package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	cmtcfg "github.com/cometbft/cometbft/config"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/syndtr/goleveldb/leveldb/util"
	"gitlab.com/thorchain/thornode/v3/app"
	"gitlab.com/thorchain/thornode/v3/common"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	snapshottypes "cosmossdk.io/store/snapshots/types"
	storetypes "cosmossdk.io/store/types"
	confixcmd "cosmossdk.io/tools/confix/cmd"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	"github.com/cosmos/cosmos-sdk/server/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	"github.com/cosmos/cosmos-sdk/types/mempool"
	"github.com/cosmos/cosmos-sdk/types/module"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"

	errorsmod "cosmossdk.io/errors"
	gogogrpc "github.com/cosmos/gogoproto/grpc"
	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	//	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"

	"gitlab.com/thorchain/thornode/v3/cmd/thornode/cmd"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	thorlog "gitlab.com/thorchain/thornode/v3/log"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/client/cli"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/forking"

	"github.com/CosmWasm/wasmd/x/wasm"
	wasmcli "github.com/CosmWasm/wasmd/x/wasm/client/cli"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

// initCometBFTConfig helps to override default CometBFT Config values.
// return cmtcfg.DefaultConfig if no custom configuration is required for the application.
func initCometBFTConfig() *cmtcfg.Config {
	cfg := cmtcfg.DefaultConfig()

	// these values put a higher strain on node memory
	// cfg.P2P.MaxNumInboundPeers = 100
	// cfg.P2P.MaxNumOutboundPeers = 40

	return cfg
}

// initAppConfig helps to override default appConfig template and configs.
// return "", nil if no custom configuration is required for the application.
func initAppConfig() (string, interface{}) {
	// The following code snippet is just for reference.

	type CustomAppConfig struct {
		serverconfig.Config
		Wasm     wasmtypes.WasmConfig    `mapstructure:"wasm"`
		EBifrost ebifrost.EBifrostConfig `mapstructure:"ebifrost"`
	}

	// Optionally allow the chain developer to overwrite the SDK's default
	// server config.
	srvCfg := serverconfig.DefaultConfig()
	// The SDK's default minimum gas price is set to "" (empty value) inside
	// app.toml. If left empty by validators, the node will halt on startup.
	// However, the chain developer can set a default app.toml value for their
	// validators here.
	//
	// In summary:
	// - if you leave srvCfg.MinGasPrices = "", all validators MUST tweak their
	//   own app.toml config,
	// - if you set srvCfg.MinGasPrices non-empty, validators CAN tweak their
	//   own app.toml to override, or use this default value.
	//
	// In simapp, we set the min gas prices to 0.
	srvCfg.MinGasPrices = "0stake"
	// srvCfg.BaseConfig.IAVLDisableFastNode = true // disable fastnode by default

	customAppConfig := CustomAppConfig{
		Config: *srvCfg,
		Wasm:   wasmtypes.DefaultWasmConfig(),
	}

	customAppTemplate := serverconfig.DefaultConfigTemplate
	customAppTemplate += wasmtypes.DefaultConfigTemplate()
	customAppTemplate += ebifrost.DefaultConfigTemplate()

	return customAppTemplate, customAppConfig
}

func initRootCmd(
	rootCmd *cobra.Command,
	txConfig client.TxConfig,
	interfaceRegistry codectypes.InterfaceRegistry,
	appCodec codec.Codec,
	basicManager module.BasicManager,
) {
	cfg := sdk.GetConfig()
	cfg.Seal()

	rootCmd.AddCommand(
		genutilcli.InitCmd(basicManager, app.DefaultNodeHome),
		//		NewTestnetCmd(basicManager, banktypes.GenesisBalancesIterator{}),
		debug.Cmd(),
		confixcmd.ConfigCommand(),
		pruning.Cmd(newApp, app.DefaultNodeHome),
		snapshot.Cmd(newApp),
		renderConfigCommand(),
		cmd.GetEd25519Keys(),
		cmd.GetPubKeyCmd(),
	)

	server.AddCommands(rootCmd, app.DefaultNodeHome, newApp, appExport, addModuleInitFlags)
	wasmcli.ExtendUnsafeResetAllCmd(rootCmd)

	// add keybase, auxiliary RPC, query, genesis, and tx child commands
	rootCmd.AddCommand(
		server.StatusCommand(),
		genesisCommand(txConfig, basicManager),
		queryCommand(),
		txCommand(),
		cli.GetUtilCmd(),
		compactCommand(),
		keys.Commands(),
	)
}

func addModuleInitFlags(startCmd *cobra.Command) {
	startCmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		serverCtx := server.GetServerContextFromCmd(cmd)

		// Bind flags to the Context's Viper so the app construction can set
		// options accordingly.
		if err := serverCtx.Viper.BindPFlags(cmd.Flags()); err != nil {
			return fmt.Errorf("fail to bind flags,err: %w", err)
		}

		// replace sdk logger with thorlog
		if zl, ok := serverCtx.Logger.Impl().(*zerolog.Logger); ok {
			logger := zl.With().CallerWithSkipFrameCount(3).Logger()
			serverCtx.Logger = thorlog.SdkLogWrapper{
				Logger: &logger,
			}
			return server.SetCmdServerContext(startCmd, serverCtx)
		}
		return nil
	}
	wasm.AddModuleInitFlags(startCmd)
	ebifrost.AddModuleInitFlags(startCmd)
	forking.AddModuleInitFlags(startCmd)
}

func renderConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:                        "render-config",
		Short:                      "renders tendermint and cosmos config from thornode base config",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		Run: func(cmd *cobra.Command, args []string) {
			config.Init()
			config.InitThornode(cmd.Context())
		},
	}
}

func queryMappedAddress() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "address [address]",
		Short: "Convert an address to its mapped bech32 format",
		Long:  ``,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			address, err := common.NewAddress(args[0])
			if err != nil {
				return fmt.Errorf("failed to read address: %w", err)
			}

			bech32Addr, err := address.MappedAccAddress()
			if err != nil {
				return fmt.Errorf("failed to convert to bech32: %w", err)
			}

			fmt.Println(bech32Addr.String())
			return nil
		},
	}
	return cmd
}

// genesisCommand builds genesis-related `simd genesis` command. Users may provide application specific commands as a parameter
func genesisCommand(txConfig client.TxConfig, basicManager module.BasicManager, cmds ...*cobra.Command) *cobra.Command {
	cmd := genutilcli.Commands(txConfig, basicManager, app.DefaultNodeHome)

	for _, subCmd := range cmds {
		cmd.AddCommand(subCmd)
	}
	return cmd
}

func queryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "query",
		Aliases:                    []string{"q"},
		Short:                      "Querying subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		rpc.QueryEventForTxCmd(),
		server.QueryBlockCmd(),
		authcmd.QueryTxsByEventsCmd(),
		server.QueryBlocksCmd(),
		authcmd.QueryTxCmd(),
		server.QueryBlockResultsCmd(),
		queryMappedAddress(),
	)

	return cmd
}

func txCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "Transactions subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetMultiSignBatchCmd(),
		authcmd.GetValidateSignaturesCommand(),
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		authcmd.GetSimulateCmd(),
	)

	return cmd
}

func compactCommand() *cobra.Command {
	return &cobra.Command{
		Use:                        "compact",
		Short:                      "force leveldb compaction",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE: func(cmd *cobra.Command, args []string) error {
			data := filepath.Join(app.DefaultNodeHome, "data")
			db, err := dbm.NewGoLevelDB("application", data, nil)
			if err != nil {
				return err
			}
			return db.DB().CompactRange(util.Range{})
		},
	}
}

// newApp creates the application
func newApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts servertypes.AppOptions,
) servertypes.Application {
	baseappOptions := DefaultBaseappOptions(appOpts)

	var wasmOpts []wasmkeeper.Option
	if cast.ToBool(appOpts.Get("telemetry.enabled")) {
		wasmOpts = append(wasmOpts, wasmkeeper.WithVMCacheMetrics(prometheus.DefaultRegisterer))
	}

	return app.NewChainApp(
		logger, db, traceStore, true,
		appOpts,
		wasmOpts,
		baseappOptions...,
	)
}

// appExport creates a new wasm app (optionally at a given height) and exports state.
func appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	var chainApp *app.THORChainApp
	// this check is necessary as we use the flag in x/upgrade.
	// we can exit more gracefully by checking the flag here.
	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home is not set")
	}

	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}

	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(server.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts
	var emptyWasmOpts []wasmkeeper.Option

	chainApp = app.NewChainApp(
		logger,
		db,
		traceStore,
		height == -1,
		appOpts,
		emptyWasmOpts,
	)

	if height != -1 {
		if err := chainApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	}

	return chainApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}

// DefaultBaseappOptions returns the default baseapp options provided by the Cosmos SDK
func DefaultBaseappOptions(appOpts types.AppOptions) []func(*baseapp.BaseApp) {
	var cache storetypes.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(server.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	pruningOpts, err := server.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	homeDir := cast.ToString(appOpts.Get(flags.FlagHome))
	chainID := cast.ToString(appOpts.Get(flags.FlagChainID))
	if chainID == "" {
		// fallback to genesis chain-id
		reader, err2 := os.Open(filepath.Join(homeDir, "config", "genesis.json"))
		if err2 != nil {
			panic(err2)
		}
		defer reader.Close()

		chainID, err = genutiltypes.ParseChainIDFromGenesis(reader)
		if err != nil {
			panic(fmt.Errorf("failed to parse chain-id from genesis file: %w", err))
		}
	}

	snapshotStore, err := server.GetSnapshotStore(appOpts)
	if err != nil {
		panic(err)
	}

	snapshotOptions := snapshottypes.NewSnapshotOptions(
		cast.ToUint64(appOpts.Get(server.FlagStateSyncSnapshotInterval)),
		cast.ToUint32(appOpts.Get(server.FlagStateSyncSnapshotKeepRecent)),
	)

	defaultMempool := baseapp.SetMempool(mempool.NoOpMempool{})
	if maxTxs := cast.ToInt(appOpts.Get(server.FlagMempoolMaxTxs)); maxTxs >= 0 {
		defaultMempool = baseapp.SetMempool(
			mempool.NewSenderNonceMempool(
				mempool.SenderNonceMaxTxOpt(maxTxs),
			),
		)
	}

	return []func(*baseapp.BaseApp){
		baseapp.SetPruning(pruningOpts),
		// baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(server.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(server.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(server.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(server.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(server.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(server.FlagIndexEvents))),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(cast.ToInt(appOpts.Get(server.FlagIAVLCacheSize))),
		baseapp.SetIAVLDisableFastNode(cast.ToBool(appOpts.Get(server.FlagDisableIAVLFastNode))),
		defaultMempool,
		baseapp.SetChainID(chainID),
		setCustomGRPCInterceptor(),
		// baseapp.SetQueryGasLimit(cast.ToUint64(appOpts.Get(server.FlagQueryGasLimit))),
	}
}

// setCustomGRPCInterceptor returns a BaseApp option that sets up a custom gRPC interceptor
func setCustomGRPCInterceptor() func(*baseapp.BaseApp) {
	return func(app *baseapp.BaseApp) {
		originalRegisterGRPCServer := app.RegisterGRPCServer
		app.RegisterGRPCServer = func(server gogogrpc.Server) {
			interceptor := func(grpcCtx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
				md, ok := metadata.FromIncomingContext(grpcCtx)
				if !ok {
					return nil, status.Error(codes.Internal, "unable to retrieve metadata")
				}

				var height int64
				if heightHeaders := md.Get(grpctypes.GRPCBlockHeightHeader); len(heightHeaders) == 1 {
					height, err = strconv.ParseInt(heightHeaders[0], 10, 64)
					if err != nil {
						return nil, errorsmod.Wrapf(
							sdkerrors.ErrInvalidRequest,
							"Custom gRPC interceptor: invalid height header %q: %v", grpctypes.GRPCBlockHeightHeader, err)
					}
				}

				sdkCtx, err := app.CreateQueryContextWithCheckHeader(height, false, true)
				if err != nil {
					return nil, err
				}

				sdkCtx = sdkCtx.WithContext(context.WithValue(sdkCtx.Context(), constants.CtxUserAPICall, true))

				if height == 0 {
					height = sdkCtx.BlockHeight()
				}

				grpcCtx = context.WithValue(grpcCtx, sdk.SdkContextKey, sdkCtx)

				md = metadata.Pairs(grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(height, 10))
				if err = grpc.SetHeader(grpcCtx, md); err != nil {
				}

				defer func() {
					if r := recover(); r != nil {
						switch rType := r.(type) {
						case storetypes.ErrorOutOfGas:
							err = errorsmod.Wrapf(sdkerrors.ErrOutOfGas, "Query gas limit exceeded: %v, out of gas in location: %v", sdkCtx.GasMeter().Limit(), rType.Descriptor)
						default:
							panic(r)
						}
					}
				}()

				return handler(grpcCtx, req)
			}

			for _, data := range app.GRPCQueryRouter().serviceData {
				desc := data.serviceDesc
				newMethods := make([]grpc.MethodDesc, len(desc.Methods))

				for i, method := range desc.Methods {
					methodHandler := method.Handler
					newMethods[i] = grpc.MethodDesc{
						MethodName: method.MethodName,
						Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
							return methodHandler(srv, ctx, dec, grpcmiddleware.ChainUnaryServer(
								grpcrecovery.UnaryServerInterceptor(),
								interceptor,
							))
						},
					}
				}

				newDesc := &grpc.ServiceDesc{
					ServiceName: desc.ServiceName,
					HandlerType: desc.HandlerType,
					Methods:     newMethods,
					Streams:     desc.Streams,
					Metadata:    desc.Metadata,
				}

				server.RegisterService(newDesc, data.handler)
			}
		}
	}
}

var tempDir = func() string {
	dir, err := os.MkdirTemp("", "simd")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	return dir
}
