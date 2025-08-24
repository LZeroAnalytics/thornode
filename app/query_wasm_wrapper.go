package app

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

type WasmQueryWrapper struct {
	wasmtypes.UnimplementedQueryServer
	app      *THORChainApp
	original wasmtypes.QueryServer
	keeper   *wasmkeeper.Keeper
}

func NewWasmQueryWrapper(app *THORChainApp, k *wasmkeeper.Keeper, original wasmtypes.QueryServer) *WasmQueryWrapper {
	return &WasmQueryWrapper{
		app:      app,
		original: original,
		keeper:   k,
	}
}

func (w *WasmQueryWrapper) SmartContractState(goCtx context.Context, req *wasmtypes.QuerySmartContractStateRequest) (*wasmtypes.QuerySmartContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if ci := w.keeper.GetContractInfo(ctx, sdk.MustAccAddressFromBech32(req.Address)); ci != nil {
		_ = w.app.materializeAndPinWasm(ctx, ci.CodeID)
	}
	return w.original.SmartContractState(goCtx, req)
}

func (w *WasmQueryWrapper) RawContractState(goCtx context.Context, req *wasmtypes.QueryRawContractStateRequest) (*wasmtypes.QueryRawContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if ci := w.keeper.GetContractInfo(ctx, sdk.MustAccAddressFromBech32(req.Address)); ci != nil {
		_ = w.app.materializeAndPinWasm(ctx, ci.CodeID)
	}
	return w.original.RawContractState(goCtx, req)
}

func (w *WasmQueryWrapper) Code(ctx context.Context, req *wasmtypes.QueryCodeRequest) (*wasmtypes.QueryCodeResponse, error) {
	return w.original.Code(ctx, req)
}
func (w *WasmQueryWrapper) Codes(ctx context.Context, req *wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) {
	return w.original.Codes(ctx, req)
}
func (w *WasmQueryWrapper) PinnedCodes(ctx context.Context, req *wasmtypes.QueryPinnedCodesRequest) (*wasmtypes.QueryPinnedCodesResponse, error) {
	return w.original.PinnedCodes(ctx, req)
}
func (w *WasmQueryWrapper) ContractInfo(ctx context.Context, req *wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
	return w.original.ContractInfo(ctx, req)
}
func (w *WasmQueryWrapper) ContractHistory(ctx context.Context, req *wasmtypes.QueryContractHistoryRequest) (*wasmtypes.QueryContractHistoryResponse, error) {
	return w.original.ContractHistory(ctx, req)
}
func (w *WasmQueryWrapper) ContractsByCode(ctx context.Context, req *wasmtypes.QueryContractsByCodeRequest) (*wasmtypes.QueryContractsByCodeResponse, error) {
	return w.original.ContractsByCode(ctx, req)
}
func (w *WasmQueryWrapper) AllContractState(ctx context.Context, req *wasmtypes.QueryAllContractStateRequest) (*wasmtypes.QueryAllContractStateResponse, error) {
	return w.original.AllContractState(ctx, req)
}
