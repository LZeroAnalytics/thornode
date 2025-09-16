package forking

import (
	"time"

	"github.com/spf13/cobra"
)

const (
	FlagForkGRPC            = "fork.grpc"
	FlagForkChainID         = "fork.chain-id"
	FlagForkHeight          = "fork.height"
	FlagForkTrustHeight     = "fork.trust-height"
	FlagForkTrustHash       = "fork.trust-hash"
	FlagForkTrustingPeriod  = "fork.trusting-period"
	FlagForkMaxClockDrift   = "fork.max-clock-drift"
	FlagForkTimeout         = "fork.timeout"
	FlagForkCacheEnabled    = "fork.cache-enabled"
	FlagForkCacheSize       = "fork.cache-size"
	FlagForkGasCostPerFetch = "fork.gas-cost-per-fetch"
)

func AddModuleInitFlags(startCmd *cobra.Command) {
	startCmd.PersistentFlags().String(FlagForkGRPC, "", "Remote gRPC endpoint for forking (e.g., thornode.ninerealms.com:9090)")
	startCmd.PersistentFlags().String(FlagForkChainID, "", "Chain ID of the remote chain to fork from (e.g., thorchain-1)")
	startCmd.PersistentFlags().Int64(FlagForkHeight, 0, "Block height to fork from (0 = latest block)")
	startCmd.PersistentFlags().Int64(FlagForkTrustHeight, 0, "Trusted block height for light client verification (0 = auto-detect)")
	startCmd.PersistentFlags().String(FlagForkTrustHash, "", "Trusted block hash for light client verification (empty = auto-detect)")
	startCmd.PersistentFlags().Duration(FlagForkTrustingPeriod, 24*time.Hour, "Trusting period for light client verification")
	startCmd.PersistentFlags().Duration(FlagForkMaxClockDrift, 10*time.Second, "Maximum allowed clock drift for header verification")
	startCmd.PersistentFlags().Duration(FlagForkTimeout, 30*time.Second, "Timeout for remote gRPC calls")
	startCmd.PersistentFlags().Bool(FlagForkCacheEnabled, true, "Enable caching of remote state")
	startCmd.PersistentFlags().Int(FlagForkCacheSize, 10000, "Maximum number of entries in the cache")
	startCmd.PersistentFlags().Uint64(FlagForkGasCostPerFetch, 1000, "Gas cost charged per remote fetch operation")
}
