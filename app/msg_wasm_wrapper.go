package app

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

type WasmMsgWrapper struct {
	wasmtypes.UnimplementedMsgServer
	app      *THORChainApp
	original wasmtypes.MsgServer
	keeper   *wasmkeeper.Keeper
}

func NewWasmMsgWrapper(app *THORChainApp, k *wasmkeeper.Keeper, original wasmtypes.MsgServer) *WasmMsgWrapper {
	return &WasmMsgWrapper{
		app:      app,
		original: original,
		keeper:   k,
	}
}

func (w *WasmMsgWrapper) withMaterialized(goCtx context.Context, codeID uint64) (context.Context, error) {
	if codeID != 0 {
		if err := w.app.materializeAndPinWasm(sdk.UnwrapSDKContext(goCtx), codeID); err != nil {
			return goCtx, err
		}
	}
	return goCtx, nil
}

func (w *WasmMsgWrapper) ExecuteContract(goCtx context.Context, req *wasmtypes.MsgExecuteContract) (*wasmtypes.MsgExecuteContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	contractAddr := sdk.MustAccAddressFromBech32(req.Contract)
	if ci := w.keeper.GetContractInfo(ctx, contractAddr); ci != nil {
		if _, err := w.withMaterialized(goCtx, ci.CodeID); err != nil {
		}
	}
	return w.original.ExecuteContract(goCtx, req)
}

func (w *WasmMsgWrapper) InstantiateContract(goCtx context.Context, req *wasmtypes.MsgInstantiateContract) (*wasmtypes.MsgInstantiateContractResponse, error) {
	if _, err := w.withMaterialized(goCtx, req.CodeID); err != nil {
	}
	return w.original.InstantiateContract(goCtx, req)
}

func (w *WasmMsgWrapper) InstantiateContract2(goCtx context.Context, req *wasmtypes.MsgInstantiateContract2) (*wasmtypes.MsgInstantiateContract2Response, error) {
	if _, err := w.withMaterialized(goCtx, req.CodeID); err != nil {
	}
	return w.original.InstantiateContract2(goCtx, req)
}

func (w *WasmMsgWrapper) MigrateContract(goCtx context.Context, req *wasmtypes.MsgMigrateContract) (*wasmtypes.MsgMigrateContractResponse, error) {
	if _, err := w.withMaterialized(goCtx, req.CodeID); err != nil {
	}
	return w.original.MigrateContract(goCtx, req)
}

func (w *WasmMsgWrapper) UpdateAdmin(ctx context.Context, req *wasmtypes.MsgUpdateAdmin) (*wasmtypes.MsgUpdateAdminResponse, error) {
	return w.original.UpdateAdmin(ctx, req)
}
func (w *WasmMsgWrapper) ClearAdmin(ctx context.Context, req *wasmtypes.MsgClearAdmin) (*wasmtypes.MsgClearAdminResponse, error) {
	return w.original.ClearAdmin(ctx, req)
}
func (w *WasmMsgWrapper) UpdateContractLabel(ctx context.Context, req *wasmtypes.MsgUpdateContractLabel) (*wasmtypes.MsgUpdateContractLabelResponse, error) {
	return w.original.UpdateContractLabel(ctx, req)
}
func (w *WasmMsgWrapper) StoreCode(ctx context.Context, req *wasmtypes.MsgStoreCode) (*wasmtypes.MsgStoreCodeResponse, error) {
	return w.original.StoreCode(ctx, req)
}
func (w *WasmMsgWrapper) RemoveCodeUploadParamsAddresses(ctx context.Context, req *wasmtypes.MsgRemoveCodeUploadParamsAddresses) (*wasmtypes.MsgRemoveCodeUploadParamsAddressesResponse, error) {
	return w.original.RemoveCodeUploadParamsAddresses(ctx, req)
}
func (w *WasmMsgWrapper) AddCodeUploadParamsAddresses(ctx context.Context, req *wasmtypes.MsgAddCodeUploadParamsAddresses) (*wasmtypes.MsgAddCodeUploadParamsAddressesResponse, error) {
	return w.original.AddCodeUploadParamsAddresses(ctx, req)
}
func (w *WasmMsgWrapper) UpdateInstantiateConfig(ctx context.Context, req *wasmtypes.MsgUpdateInstantiateConfig) (*wasmtypes.MsgUpdateInstantiateConfigResponse, error) {
	return w.original.UpdateInstantiateConfig(ctx, req)
}
func (w *WasmMsgWrapper) UpdateParams(ctx context.Context, req *wasmtypes.MsgUpdateParams) (*wasmtypes.MsgUpdateParamsResponse, error) {
	return w.original.UpdateParams(ctx, req)
}
func (w *WasmMsgWrapper) SudoContract(ctx context.Context, req *wasmtypes.MsgSudoContract) (*wasmtypes.MsgSudoContractResponse, error) {
	return w.original.SudoContract(ctx, req)
}
func (w *WasmMsgWrapper) PinCodes(ctx context.Context, req *wasmtypes.MsgPinCodes) (*wasmtypes.MsgPinCodesResponse, error) {
	return w.original.PinCodes(ctx, req)
}
func (w *WasmMsgWrapper) UnpinCodes(ctx context.Context, req *wasmtypes.MsgUnpinCodes) (*wasmtypes.MsgUnpinCodesResponse, error) {
	return w.original.UnpinCodes(ctx, req)
}
func (w *WasmMsgWrapper) StoreAndInstantiateContract(ctx context.Context, req *wasmtypes.MsgStoreAndInstantiateContract) (*wasmtypes.MsgStoreAndInstantiateContractResponse, error) {
	return w.original.StoreAndInstantiateContract(ctx, req)
}
func (w *WasmMsgWrapper) StoreAndMigrateContract(ctx context.Context, req *wasmtypes.MsgStoreAndMigrateContract) (*wasmtypes.MsgStoreAndMigrateContractResponse, error) {
	return w.original.StoreAndMigrateContract(ctx, req)
}

func contractAddrFromString(s string) []byte {
	addr, _ := sdk.AccAddressFromBech32(s)
	return address.MustLengthPrefix(addr)
}
