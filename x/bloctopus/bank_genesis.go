package bloctopus

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
)

func InitBankGenesis(ctx context.Context, keeper bankkeeper.BaseKeeper, genState *banktypes.GenesisState) {
	if err := keeper.SetParams(ctx, genState.Params); err != nil {
		panic(err)
	}

	for _, se := range genState.GetAllSendEnabled() {
		keeper.SetSendEnabled(ctx, se.Denom, se.Enabled)
	}
	totalSupplyMap := sdk.NewMapCoins(sdk.Coins{})

	genState.Balances = banktypes.SanitizeGenesisBalances(genState.Balances)

	for _, balance := range genState.Balances {
		addr := balance.GetAddress()
		
		_, addrBytes, err := bech32.DecodeAndConvert(addr)
		if err != nil {
			continue
		}

		for _, coin := range balance.Coins {
			err := keeper.Balances.Set(ctx, collections.Join(sdk.AccAddress(addrBytes), coin.Denom), coin.Amount)
			if err != nil {
				panic(err)
			}
		}

		totalSupplyMap.Add(balance.Coins...)
	}
	totalSupply := totalSupplyMap.ToCoins()

	if !genState.Supply.Empty() && !genState.Supply.Equal(totalSupply) {
		panic(fmt.Errorf("genesis supply is incorrect, expected %v, got %v", genState.Supply, totalSupply))
	}

	for _, supply := range totalSupply {
		if !supply.IsZero() {
			_ = keeper.Supply.Set(ctx, supply.Denom, supply.Amount)
		}
	}

	for _, meta := range genState.DenomMetadata {
		keeper.SetDenomMetaData(ctx, meta)
	}
}
