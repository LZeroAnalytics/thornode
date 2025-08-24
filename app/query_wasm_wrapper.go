package app

import (
	"context"
	"net"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

func (w *WasmQueryWrapper) ensureMaterializedByAddress(ctx sdk.Context, bech32Addr string) {
	addr := sdk.MustAccAddressFromBech32(bech32Addr)
	if ci := w.keeper.GetContractInfo(ctx, addr); ci != nil {
		_ = w.app.materializeAndPinWasm(ctx, ci.CodeID)
		return
	}
	target := "grpc.thor.pfc.zone:443"
	useTLS := false
	normalized := strings.TrimSpace(target)
	if strings.HasPrefix(normalized, "grpcs://") || strings.HasPrefix(normalized, "https://") {
		useTLS = true
		normalized = strings.TrimPrefix(strings.TrimPrefix(normalized, "grpcs://"), "https://")
	} else {
		if _, p, e := net.SplitHostPort(normalized); e == nil && p == "443" {
			useTLS = true
		}
	}
	var dialOpt grpc.DialOption
	if useTLS {
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(nil))
	} else {
		dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	}
	conn, err := grpc.Dial(normalized, dialOpt)
	if err != nil {
		return
	}
	defer conn.Close()
	wq := wasmtypes.NewQueryClient(conn)
	resp, err := wq.ContractInfo(ctx.Context(), &wasmtypes.QueryContractInfoRequest{Address: bech32Addr})
	if err != nil || resp == nil || resp.ContractInfo.CodeID == 0 {
		return
	}
	_ = w.app.materializeAndPinWasm(ctx, resp.ContractInfo.CodeID)
}

func (w *WasmQueryWrapper) SmartContractState(goCtx context.Context, req *wasmtypes.QuerySmartContractStateRequest) (*wasmtypes.QuerySmartContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Address)
	return w.original.SmartContractState(goCtx, req)
}

func (w *WasmQueryWrapper) RawContractState(goCtx context.Context, req *wasmtypes.QueryRawContractStateRequest) (*wasmtypes.QueryRawContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Address)
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
func (w *WasmQueryWrapper) ContractInfo(goCtx context.Context, req *wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	addr := sdk.MustAccAddressFromBech32(req.Address)
	if ci := w.keeper.GetContractInfo(ctx, addr); ci != nil {
		return &wasmtypes.QueryContractInfoResponse{Address: req.Address, ContractInfo: *ci}, nil
	}
	target := "grpc.thor.pfc.zone:443"
	useTLS := false
	normalized := strings.TrimSpace(target)
	if strings.HasPrefix(normalized, "grpcs://") || strings.HasPrefix(normalized, "https://") {
		useTLS = true
		normalized = strings.TrimPrefix(strings.TrimPrefix(normalized, "grpcs://"), "https://")
	} else {
		if _, p, e := net.SplitHostPort(normalized); e == nil && p == "443" {
			useTLS = true
		}
	}
	var dialOpt grpc.DialOption
	if useTLS {
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(nil))
	} else {
		dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	}
	conn, err := grpc.Dial(normalized, dialOpt)
	if err != nil {
		return w.original.ContractInfo(goCtx, req)
	}
	defer conn.Close()
	wq := wasmtypes.NewQueryClient(conn)
	resp, err := wq.ContractInfo(goCtx, &wasmtypes.QueryContractInfoRequest{Address: req.Address})
	if err != nil || resp == nil {
		return w.original.ContractInfo(goCtx, req)
	}
	_ = w.app.materializeAndPinWasm(ctx, resp.ContractInfo.CodeID)
	return resp, nil
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
