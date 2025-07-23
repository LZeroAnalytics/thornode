package forking

import (
	"time"

	"github.com/spf13/cobra"
)

const (
	FlagForkRPC             = "fork.rpc"
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
	startCmd.Flags().String(FlagForkRPC, "", "Remote RPC endpoint for forking (e.g., https://thornode.ninerealms.com:26657)")
	startCmd.Flags().String(FlagForkChainID, "", "Chain ID of the remote chain to fork from (e.g., thorchain-1)")
	startCmd.Flags().Int64(FlagForkHeight, 0, "Block height to fork from (0 = latest block)")
	startCmd.Flags().Int64(FlagForkTrustHeight, 0, "Trusted block height for light client verification (0 = auto-detect)")
	startCmd.Flags().String(FlagForkTrustHash, "", "Trusted block hash for light client verification (empty = auto-detect)")
	startCmd.Flags().Duration(FlagForkTrustingPeriod, 24*time.Hour, "Trusting period for light client verification")
	startCmd.Flags().Duration(FlagForkMaxClockDrift, 10*time.Second, "Maximum allowed clock drift for header verification")
	startCmd.Flags().Duration(FlagForkTimeout, 30*time.Second, "Timeout for remote RPC calls")
	startCmd.Flags().Bool(FlagForkCacheEnabled, true, "Enable caching of remote state")
	startCmd.Flags().Int(FlagForkCacheSize, 10000, "Maximum number of entries in the cache")
	startCmd.Flags().Uint64(FlagForkGasCostPerFetch, 1000, "Gas cost charged per remote fetch operation")
}
