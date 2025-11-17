package keeper

import (
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

type Config struct {
	Endpoint string
}

type ForkingBankKeeper struct {
	bankkeeper.BaseKeeper
	cfg    Config
	client *RemoteClient
}
