package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type ForkingBankKeeper struct {
	base       bankkeeper.BaseKeeper
	remote     *remoteClient
	moduleName string
}

func NewForkingBankKeeper(base bankkeeper.BaseKeeper, cfg Config) (*ForkingBankKeeper, error) {
	rc, err := newRemoteClient(cfg)
	if err != nil {
		return nil, err
	}
	return &ForkingBankKeeper{base: base, remote: rc, moduleName: cfg.ModuleName}, nil
}
func (k ForkingBankKeeper) Balance(ctx context.Context, req *banktypes.QueryBalanceRequest) (*banktypes.QueryBalanceResponse, error) {
	if req == nil {
		return nil, nil
	}
	addr, err := sdk.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, err
	}
	coin := k.GetBalance(ctx, addr, req.Denom)
	return &banktypes.QueryBalanceResponse{Balance: &coin}, nil
}

func (k ForkingBankKeeper) AllBalances(ctx context.Context, req *banktypes.QueryAllBalancesRequest) (*banktypes.QueryAllBalancesResponse, error) {
	if req == nil {
		return nil, nil
	}
	addr, err := sdk.AccAddressFromBech32(req.Address)
	if err != nil {
		return nil, err
	}
	coins := k.GetAllBalances(ctx, addr)
	return &banktypes.QueryAllBalancesResponse{Balances: coins}, nil
}

func (k ForkingBankKeeper) SpendableBalances(ctx context.Context, req *banktypes.QuerySpendableBalancesRequest) (*banktypes.QuerySpendableBalancesResponse, error) {
	return k.base.SpendableBalances(ctx, req)
}

func (k ForkingBankKeeper) SpendableBalanceByDenom(ctx context.Context, req *banktypes.QuerySpendableBalanceByDenomRequest) (*banktypes.QuerySpendableBalanceByDenomResponse, error) {
	return k.base.SpendableBalanceByDenom(ctx, req)
}

func (k ForkingBankKeeper) TotalSupply(ctx context.Context, req *banktypes.QueryTotalSupplyRequest) (*banktypes.QueryTotalSupplyResponse, error) {
	return k.base.TotalSupply(ctx, req)
}

func (k ForkingBankKeeper) SupplyOf(ctx context.Context, req *banktypes.QuerySupplyOfRequest) (*banktypes.QuerySupplyOfResponse, error) {
	return k.base.SupplyOf(ctx, req)
}

func (k ForkingBankKeeper) Params(ctx context.Context, req *banktypes.QueryParamsRequest) (*banktypes.QueryParamsResponse, error) {
	return k.base.Params(ctx, req)
}

func (k ForkingBankKeeper) DenomsMetadata(ctx context.Context, req *banktypes.QueryDenomsMetadataRequest) (*banktypes.QueryDenomsMetadataResponse, error) {
	local := k.base.GetAllDenomMetaData(ctx)
	if k.remote == nil {
		return &banktypes.QueryDenomsMetadataResponse{Metadatas: local}, nil
	}
	remote, _ := k.remote.DenomsMetadata(ctx)
	return &banktypes.QueryDenomsMetadataResponse{Metadatas: mergeMetadata(local, remote)}, nil
}

func (k ForkingBankKeeper) DenomMetadata(ctx context.Context, req *banktypes.QueryDenomMetadataRequest) (*banktypes.QueryDenomMetadataResponse, error) {
	if req == nil {
		return nil, nil
	}
	if md, ok := k.base.GetDenomMetaData(ctx, req.Denom); ok {
		return &banktypes.QueryDenomMetadataResponse{Metadata: md}, nil
	}
	if k.remote == nil {
		return &banktypes.QueryDenomMetadataResponse{}, nil
	}
	md, ok := k.remote.DenomMetadata(ctx, req.Denom)
	if ok {
		return &banktypes.QueryDenomMetadataResponse{Metadata: md}, nil
	}
	return &banktypes.QueryDenomMetadataResponse{}, nil
}

func (k ForkingBankKeeper) DenomMetadataByQueryString(ctx context.Context, req *banktypes.QueryDenomMetadataByQueryStringRequest) (*banktypes.QueryDenomMetadataByQueryStringResponse, error) {
	return k.base.DenomMetadataByQueryString(ctx, req)
}

func (k ForkingBankKeeper) DenomOwners(ctx context.Context, req *banktypes.QueryDenomOwnersRequest) (*banktypes.QueryDenomOwnersResponse, error) {
	return k.base.DenomOwners(ctx, req)
}

func (k ForkingBankKeeper) SendEnabled(ctx context.Context, req *banktypes.QuerySendEnabledRequest) (*banktypes.QuerySendEnabledResponse, error) {
	return k.base.SendEnabled(ctx, req)
}

func (k ForkingBankKeeper) DenomOwnersByQuery(ctx context.Context, req *banktypes.QueryDenomOwnersByQueryRequest) (*banktypes.QueryDenomOwnersByQueryResponse, error) {
	return k.base.DenomOwnersByQuery(ctx, req)
}




func (k ForkingBankKeeper) GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	coin := k.base.GetBalance(ctx, addr, denom)
	if !coin.IsZero() {
		return coin
	}
	if k.remote == nil {
		return coin
	}
	rc, _ := k.remote.AllBalances(ctx, addr.String())
	if rc == nil {
		return coin
	}
	if amt := rc.AmountOfNoDenomValidation(denom); !amt.IsZero() {
		return sdk.NewCoin(denom, amt)
	}
	return coin
}

func (k ForkingBankKeeper) GetAllBalances(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	local := k.base.GetAllBalances(ctx, addr)
	if k.remote == nil {
		return local
	}
	rc, _ := k.remote.AllBalances(ctx, addr.String())
	if rc == nil {
		return local
	}
	return mergeCoins(local, rc)
}

func (k ForkingBankKeeper) IterateAccountBalances(ctx context.Context, addr sdk.AccAddress, cb func(sdk.Coin) bool) {
	local := k.base.GetAllBalances(ctx, addr)
	seen := map[string]struct{}{}
	for _, c := range local {
		seen[c.Denom] = struct{}{}
		if stop := cb(c); stop {
			return
		}
	}
	if k.remote == nil {
		return
	}
	rc, _ := k.remote.AllBalances(ctx, addr.String())
	for _, c := range rc {
		if _, ok := seen[c.Denom]; ok {
			continue
		}
		if stop := cb(c); stop {
			return
		}
	}
}


func (k ForkingBankKeeper) ensureMaterialized(ctx context.Context, addr sdk.AccAddress) {
	if k.remote == nil {
		return
	}
	local := k.base.GetAllBalances(ctx, addr)
	if !local.IsZero() {
		return
	}
	rc, _ := k.remote.AllBalances(ctx, addr.String())
	if rc == nil || rc.IsZero() {
		return
	}
	_ = k.base.MintCoins(ctx, k.moduleName, rc)
	_ = k.base.SendCoinsFromModuleToAccount(ctx, k.moduleName, addr, rc)
}


func (k ForkingBankKeeper) SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, fromAddr)
	k.ensureMaterialized(ctx, toAddr)
	return k.base.SendCoins(ctx, fromAddr, toAddr, amt)
}

func (k ForkingBankKeeper) SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, recipientAddr)
	return k.base.SendCoinsFromModuleToAccount(ctx, senderModule, recipientAddr, amt)
}

func (k ForkingBankKeeper) SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, senderAddr)
	return k.base.SendCoinsFromAccountToModule(ctx, senderAddr, recipientModule, amt)
}

func (k ForkingBankKeeper) DelegateCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, senderAddr)
	return k.base.DelegateCoinsFromAccountToModule(ctx, senderAddr, recipientModule, amt)
}

func (k ForkingBankKeeper) UndelegateCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, recipientAddr)
	return k.base.UndelegateCoinsFromModuleToAccount(ctx, senderModule, recipientAddr, amt)
}

func (k ForkingBankKeeper) DelegateCoins(ctx context.Context, delegatorAddr sdk.AccAddress, moduleAccAddr sdk.AccAddress, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, delegatorAddr)
	k.ensureMaterialized(ctx, moduleAccAddr)
	return k.base.DelegateCoins(ctx, delegatorAddr, moduleAccAddr, amt)
}

func (k ForkingBankKeeper) UndelegateCoins(ctx context.Context, moduleAccAddr sdk.AccAddress, delegatorAddr sdk.AccAddress, amt sdk.Coins) error {
	k.ensureMaterialized(ctx, moduleAccAddr)
	k.ensureMaterialized(ctx, delegatorAddr)
	return k.base.UndelegateCoins(ctx, moduleAccAddr, delegatorAddr, amt)
}
func (k ForkingBankKeeper) InitGenesis(ctx context.Context, gen *banktypes.GenesisState) {
	k.base.InitGenesis(ctx, gen)
}

func (k ForkingBankKeeper) ExportGenesis(ctx context.Context) *banktypes.GenesisState {
	return k.base.ExportGenesis(ctx)
}

func (k ForkingBankKeeper) GetSupply(ctx context.Context, denom string) sdk.Coin {
	return k.base.GetSupply(ctx, denom)
}

func (k ForkingBankKeeper) HasSupply(ctx context.Context, denom string) bool {
	return k.base.HasSupply(ctx, denom)
}

func (k ForkingBankKeeper) GetPaginatedTotalSupply(ctx context.Context, pagination *query.PageRequest) (sdk.Coins, *query.PageResponse, error) {
	return k.base.GetPaginatedTotalSupply(ctx, pagination)
}

func (k ForkingBankKeeper) IterateTotalSupply(ctx context.Context, cb func(sdk.Coin) bool) {
	k.base.IterateTotalSupply(ctx, cb)
}

func (k ForkingBankKeeper) HasDenomMetaData(ctx context.Context, denom string) bool {
	return k.base.HasDenomMetaData(ctx, denom)
}

func (k ForkingBankKeeper) GetAllDenomMetaData(ctx context.Context) []banktypes.Metadata {
	return k.base.GetAllDenomMetaData(ctx)
}

func (k ForkingBankKeeper) IterateAllDenomMetaData(ctx context.Context, cb func(banktypes.Metadata) bool) {
	k.base.IterateAllDenomMetaData(ctx, cb)
}

func (k ForkingBankKeeper) SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	return k.base.SendCoinsFromModuleToModule(ctx, senderModule, recipientModule, amt)
}

func (k ForkingBankKeeper) InputOutputCoins(ctx context.Context, input banktypes.Input, outputs []banktypes.Output) error {
	return k.base.InputOutputCoins(ctx, input, outputs)
}

func (k ForkingBankKeeper) GetAccountsBalances(ctx context.Context) []banktypes.Balance {
	return k.base.GetAccountsBalances(ctx)
}

func (k ForkingBankKeeper) IterateAllBalances(ctx context.Context, cb func(address sdk.AccAddress, coin sdk.Coin) bool) {
	k.base.IterateAllBalances(ctx, cb)
}

func (k ForkingBankKeeper) LockedCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	return k.base.LockedCoins(ctx, addr)
}

func (k ForkingBankKeeper) SpendableCoin(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	return k.base.SpendableCoin(ctx, addr, denom)
}

func (k ForkingBankKeeper) ValidateBalance(ctx context.Context, addr sdk.AccAddress) error {
	return k.base.ValidateBalance(ctx, addr)
}

func (k ForkingBankKeeper) HasBalance(ctx context.Context, addr sdk.AccAddress, amt sdk.Coin) bool {
	return k.base.HasBalance(ctx, addr, amt)
}


func (k ForkingBankKeeper) GetParams(ctx context.Context) banktypes.Params {
	return k.base.GetParams(ctx)
}

func (k ForkingBankKeeper) SetParams(ctx context.Context, params banktypes.Params) error {
	return k.base.SetParams(ctx, params)
}

func (k ForkingBankKeeper) GetSendEnabledEntry(ctx context.Context, denom string) (banktypes.SendEnabled, bool) {
	return k.base.GetSendEnabledEntry(ctx, denom)
}

func (k ForkingBankKeeper) GetAllSendEnabledEntries(ctx context.Context) []banktypes.SendEnabled {
	return k.base.GetAllSendEnabledEntries(ctx)
}

func (k ForkingBankKeeper) AppendSendRestriction(rfn banktypes.SendRestrictionFn) {
	k.base.AppendSendRestriction(rfn)
}
func (k ForkingBankKeeper) PrependSendRestriction(rfn banktypes.SendRestrictionFn) {
	k.base.PrependSendRestriction(rfn)
}


func (k ForkingBankKeeper) WithMintCoinsRestriction(mrf banktypes.MintingRestrictionFn) bankkeeper.BaseKeeper {
	return k.base.WithMintCoinsRestriction(mrf)
}

func (k ForkingBankKeeper) ClearSendRestriction() {
	k.base.ClearSendRestriction()
}

func (k ForkingBankKeeper) SetSendEnabled(ctx context.Context, denom string, v bool) {
	k.base.SetSendEnabled(ctx, denom, v)
}
func (k ForkingBankKeeper) SetAllSendEnabled(ctx context.Context, se []*banktypes.SendEnabled) {
	k.base.SetAllSendEnabled(ctx, se)
}


func (k ForkingBankKeeper) DeleteSendEnabled(ctx context.Context, denoms ...string) {
	k.base.DeleteSendEnabled(ctx, denoms...)
}

func (k ForkingBankKeeper) IterateSendEnabledEntries(ctx context.Context, cb func(denom string, v bool) bool) {
	k.base.IterateSendEnabledEntries(ctx, cb)
}



func (k ForkingBankKeeper) BlockedAddr(addr sdk.AccAddress) bool {
	return k.base.BlockedAddr(addr)
}

func (k ForkingBankKeeper) IsSendEnabledCoin(ctx context.Context, coin sdk.Coin) bool {
	return k.base.IsSendEnabledCoin(ctx, coin)
}

func (k ForkingBankKeeper) IsSendEnabledCoins(ctx context.Context, coins ...sdk.Coin) error {
	return k.base.IsSendEnabledCoins(ctx, coins...)
}

func (k ForkingBankKeeper) IsSendEnabledDenom(ctx context.Context, denom string) bool {
	return k.base.IsSendEnabledDenom(ctx, denom)
}


func (k ForkingBankKeeper) GetDenomMetaData(ctx context.Context, denom string) (banktypes.Metadata, bool) {
	return k.base.GetDenomMetaData(ctx, denom)
}

func (k ForkingBankKeeper) SetDenomMetaData(ctx context.Context, denomMetaData banktypes.Metadata) {
	k.base.SetDenomMetaData(ctx, denomMetaData)
}

func (k ForkingBankKeeper) GetBlockedAddresses() map[string]bool {
	return k.base.GetBlockedAddresses()
}

func (k ForkingBankKeeper) GetAuthority() string {
	return k.base.GetAuthority()
}

func (k ForkingBankKeeper) GetModuleAddress(name string) sdk.AccAddress {
	return authtypes.NewModuleAddress(name)
}


func (k ForkingBankKeeper) MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error {
	return k.base.MintCoins(ctx, moduleName, amt)
}

func (k ForkingBankKeeper) BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error {
	return k.base.BurnCoins(ctx, moduleName, amt)
}

func (k ForkingBankKeeper) SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	loc := k.base.SpendableCoins(ctx, addr)
	if loc.IsZero() && k.remote != nil {
		rc, _ := k.remote.AllBalances(ctx, addr.String())
		return rc
	}
	return loc
}
