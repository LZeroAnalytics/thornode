package thorchain

import (
	"context"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"google.golang.org/grpc/metadata"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/constants"
)

func (ms msgServer) StoreCode(goCtx context.Context, msg *wasmtypes.MsgStoreCode) (*wasmtypes.MsgStoreCodeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmStoreCodeHandler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) InstantiateContract(goCtx context.Context, msg *wasmtypes.MsgInstantiateContract) (*wasmtypes.MsgInstantiateContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmInstantiateContractHandler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) InstantiateContract2(goCtx context.Context, msg *wasmtypes.MsgInstantiateContract2) (*wasmtypes.MsgInstantiateContract2Response, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmInstantiateContract2Handler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) ExecuteContract(goCtx context.Context, msg *wasmtypes.MsgExecuteContract) (*wasmtypes.MsgExecuteContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmExecuteContractHandler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) MigrateContract(goCtx context.Context, msg *wasmtypes.MsgMigrateContract) (*wasmtypes.MsgMigrateContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmMigrateContractHandler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) SudoContract(goCtx context.Context, msg *wasmtypes.MsgSudoContract) (*wasmtypes.MsgSudoContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmSudoContractHandler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) UpdateAdmin(goCtx context.Context, msg *wasmtypes.MsgUpdateAdmin) (*wasmtypes.MsgUpdateAdminResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmUpdateAdminHandler(ms.mgr).Run(ctx, msg)
}

func (ms msgServer) ClearAdmin(goCtx context.Context, msg *wasmtypes.MsgClearAdmin) (*wasmtypes.MsgClearAdminResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return NewWasmClearAdminHandler(ms.mgr).Run(ctx, msg)
}
