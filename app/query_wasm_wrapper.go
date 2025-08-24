package app

import (
	"context"
	"fmt"
	"net"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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
	target := w.app.forkGRPC
	if strings.TrimSpace(target) == "" {
		target = "grpc.thor.pfc.zone:443"
	}
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
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(ctx.Context(), md)
	resp, err := wq.ContractInfo(qctx, &wasmtypes.QueryContractInfoRequest{Address: bech32Addr})
	if err != nil || resp == nil || resp.ContractInfo.CodeID == 0 {
		return
	}
	_ = w.app.materializeAndPinWasm(ctx, resp.ContractInfo.CodeID)
}

func (w *WasmQueryWrapper) SmartContractState(goCtx context.Context, req *wasmtypes.QuerySmartContractStateRequest) (*wasmtypes.QuerySmartContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Address)
	resp, err := w.original.SmartContractState(goCtx, req)
	if err == nil && resp != nil {
		return resp, nil
	}
	target := w.app.forkGRPC
	if strings.TrimSpace(target) == "" {
		target = "grpc.thor.pfc.zone:443"
	}
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
	conn, derr := grpc.Dial(normalized, dialOpt)
	if derr != nil {
		return resp, err
	}
	defer conn.Close()
	wq := wasmtypes.NewQueryClient(conn)
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(goCtx, md)
	return wq.SmartContractState(qctx, req)
}

func (w *WasmQueryWrapper) RawContractState(goCtx context.Context, req *wasmtypes.QueryRawContractStateRequest) (*wasmtypes.QueryRawContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Address)
	resp, err := w.original.RawContractState(goCtx, req)
	if err == nil && resp != nil {
		return resp, nil
	}
	target := w.app.forkGRPC
	if strings.TrimSpace(target) == "" {
		target = "grpc.thor.pfc.zone:443"
	}
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
	conn, derr := grpc.Dial(normalized, dialOpt)
	if derr != nil {
		return resp, err
	}
	defer conn.Close()
	wq := wasmtypes.NewQueryClient(conn)
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(goCtx, md)
	return wq.RawContractState(qctx, req)
}

func (w *WasmQueryWrapper) Code(goCtx context.Context, req *wasmtypes.QueryCodeRequest) (*wasmtypes.QueryCodeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	resp, err := w.original.Code(goCtx, req)
	if err == nil && resp != nil && len(resp.Data) > 0 {
		_ = w.app.materializeAndPinWasm(ctx, req.CodeId)
		return resp, nil
	}
	target := w.app.forkGRPC
	if strings.TrimSpace(target) == "" {
		target = "grpc.thor.pfc.zone:443"
	}
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
	conn, derr := grpc.Dial(normalized, dialOpt)
	if derr != nil {
		return resp, err
	}
	defer conn.Close()
	wq := wasmtypes.NewQueryClient(conn)
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(goCtx, md)
	rem, rerr := wq.Code(qctx, req)
	if rerr == nil && rem != nil && len(rem.Data) > 0 {
		_ = w.app.materializeAndPinWasm(ctx, req.CodeId)
		return rem, nil
	}
	return resp, err
}

func (w *WasmQueryWrapper) CodeInfo(goCtx context.Context, req *wasmtypes.QueryCodeRequest) (*wasmtypes.QueryCodeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	resp, err := w.original.Code(goCtx, req)
	if err == nil && resp != nil {
		return resp, nil
	}
	target := w.app.forkGRPC
	if strings.TrimSpace(target) == "" {
		target = "grpc.thor.pfc.zone:443"
	}
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
	conn, derr := grpc.Dial(normalized, dialOpt)
	if derr != nil {
		return resp, err
	}
	defer conn.Close()
	wq := wasmtypes.NewQueryClient(conn)
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(goCtx, md)
	rem, rerr := wq.Code(qctx, req)
	if rerr == nil && rem != nil {
		_ = w.app.materializeAndPinWasm(ctx, req.CodeId)
		return rem, nil
	}
	return resp, err
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
	target := w.app.forkGRPC
	if strings.TrimSpace(target) == "" {
		target = "grpc.thor.pfc.zone:443"
	}
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
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(goCtx, md)
	resp, err := wq.ContractInfo(qctx, &wasmtypes.QueryContractInfoRequest{Address: req.Address})
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
