package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func NewForkingBankKeeper(base bankkeeper.BaseKeeper, cfg Config) (ForkingBankKeeper, error) {
	client, err := NewRemoteClient(cfg.Endpoint)
	if err != nil {
		return ForkingBankKeeper{}, err
	}
	return ForkingBankKeeper{
		BaseKeeper: base,
		cfg:        cfg,
		client:     client,
	}, nil
}

func (k ForkingBankKeeper) GetAllBalances(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	if k.BaseKeeper.HasBalance(ctx, addr, sdk.NewCoin("rune", math.NewInt(1))) {
		return k.BaseKeeper.GetAllBalances(ctx, addr)
	}
	resp, err := k.client.RemoteBalances(ctx, addr.String())
	if err != nil || resp == nil {
		return sdk.NewCoins()
	}
	return sdk.NewCoins(resp.Balances...)
}

func (k ForkingBankKeeper) GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	if k.BaseKeeper.HasBalance(ctx, addr, sdk.NewCoin(denom, math.NewInt(1))) {
		return k.BaseKeeper.GetBalance(ctx, addr, denom)
	}
	resp, err := k.client.RemoteBalances(ctx, addr.String())
	if err != nil {
		return sdk.NewCoin(denom, math.ZeroInt())
	}
	for _, c := range resp.Balances {
		if c.Denom == denom {
			return c
		}
	}
	return sdk.NewCoin(denom, math.ZeroInt())
}

func (k ForkingBankKeeper) SendCoins(ctx context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error {
	return k.BaseKeeper.SendCoins(ctx, fromAddr, toAddr, amt)
}

func (k ForkingBankKeeper) GetDenomMetaData(ctx context.Context, denom string) (banktypes.Metadata, bool) {
	md, found := k.BaseKeeper.GetDenomMetaData(ctx, denom)
	if found {
		return md, true
	}
	k.ensureDenomMetadata(ctx)
	return k.BaseKeeper.GetDenomMetaData(ctx, denom)
}
func (k ForkingBankKeeper) ensureDenomMetadata(ctx context.Context) {
	all := k.BaseKeeper.GetAllDenomMetaData(ctx)
	if len(all) > 0 {
		return
	}
	resp, err := k.client.RemoteDenomsMetadata(ctx)
	if err != nil || resp == nil || len(resp.Metadatas) == 0 {
		return
	}
	for _, m := range resp.Metadatas {
		k.BaseKeeper.SetDenomMetaData(ctx, m)
	}
}
func (k ForkingBankKeeper) EnsureDenomMetadata(ctx context.Context) {
	k.ensureDenomMetadata(ctx)
}



func (k ForkingBankKeeper) GetAllDenomMetaData(ctx context.Context) []banktypes.Metadata {
	all := k.BaseKeeper.GetAllDenomMetaData(ctx)
	if len(all) > 0 {
		return all
	}
	resp, err := k.client.RemoteDenomsMetadata(ctx)
	if err != nil || resp == nil || len(resp.Metadatas) == 0 {
		return nil
	}
	for _, m := range resp.Metadatas {
		k.BaseKeeper.SetDenomMetaData(ctx, m)
	}
	return k.BaseKeeper.GetAllDenomMetaData(ctx)
}
