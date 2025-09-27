package keeper

import "time"

type Config struct {
	GRPCEndpoint string
	ChainID      string
	Timeout      time.Duration
	ModuleName   string
}
