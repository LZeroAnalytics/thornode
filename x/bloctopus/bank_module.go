package bloctopus

import (
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
)

type BankModule struct {
	bank.AppModule
	keeper bankkeeper.BaseKeeper
}

func NewBankModule(cdc codec.Codec, keeper bankkeeper.BaseKeeper, ak banktypes.AccountKeeper, ss paramstypes.Subspace) BankModule {
	return BankModule{
		AppModule: bank.NewAppModule(cdc, keeper, ak, ss),
		keeper:    keeper,
	}
}

func (am BankModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState banktypes.GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)

	InitBankGenesis(ctx, am.keeper, &genesisState)
	return []abci.ValidatorUpdate{}
}
