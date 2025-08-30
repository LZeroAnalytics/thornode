package app

import (
	"context"
	"crypto/tls"
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

func shouldRetryWithoutHeight(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid height") {
		return true
	}
	if strings.Contains(msg, "version mismatch") {
		return true
	}
	if strings.Contains(msg, "pruned") {
		return true
	}
	return false
}

func isUserAPICall(goCtx context.Context) bool {
	return true
}

func (w *WasmQueryWrapper) ensureMaterializedByAddress(ctx sdk.Context, bech32Addr string, allowRemote bool) {
	addr := sdk.MustAccAddressFromBech32(bech32Addr)
	if ci := w.keeper.GetContractInfo(ctx, addr); ci != nil {
		_ = w.app.materializeAndPinWasm(ctx, ci.CodeID)
		return
	}
	if !allowRemote {
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	resp, rerr := wq.ContractInfo(qctx, &wasmtypes.QueryContractInfoRequest{Address: bech32Addr})
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		resp, rerr = wq.ContractInfo(ctx.Context(), &wasmtypes.QueryContractInfoRequest{Address: bech32Addr})
	}
	if rerr != nil || resp == nil || resp.ContractInfo.CodeID == 0 {
		return
	}
	_ = w.app.materializeAndPinWasm(ctx, resp.ContractInfo.CodeID)
}

func (w *WasmQueryWrapper) SmartContractState(goCtx context.Context, req *wasmtypes.QuerySmartContractStateRequest) (*wasmtypes.QuerySmartContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Address, isUserAPICall(goCtx))
	resp, err := w.original.SmartContractState(goCtx, req)
	if err == nil && resp != nil {
		return resp, nil
	}
	if !isUserAPICall(goCtx) {
		return resp, err
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	rem, rerr := wq.SmartContractState(qctx, req)
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.SmartContractState(goCtx, req)
	}
	if rerr == nil && rem != nil {
		return rem, nil
	}
	return resp, err
}

func (w *WasmQueryWrapper) RawContractState(goCtx context.Context, req *wasmtypes.QueryRawContractStateRequest) (*wasmtypes.QueryRawContractStateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Address, isUserAPICall(goCtx))
	resp, err := w.original.RawContractState(goCtx, req)
	if err == nil && resp != nil {
		return resp, nil
	}
	if !isUserAPICall(goCtx) {
		return resp, err
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	rem, rerr := wq.RawContractState(qctx, req)
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.RawContractState(goCtx, req)
	}
	if rerr == nil && rem != nil {
		return rem, nil
	}
	return resp, err
}

func (w *WasmQueryWrapper) Code(goCtx context.Context, req *wasmtypes.QueryCodeRequest) (*wasmtypes.QueryCodeResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	resp, err := w.original.Code(goCtx, req)
	if err == nil && resp != nil && len(resp.Data) > 0 {
		_ = w.app.materializeAndPinWasm(ctx, req.CodeId)
		return resp, nil
	}
	if !isUserAPICall(goCtx) {
		return resp, err
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.Code(goCtx, req)
	}
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
	if !isUserAPICall(goCtx) {
		return resp, err
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.Code(goCtx, req)
	}
	if rerr == nil && rem != nil {
		_ = w.app.materializeAndPinWasm(ctx, req.CodeId)
		return rem, nil
	}
	return resp, err
}

func (w *WasmQueryWrapper) Codes(goCtx context.Context, req *wasmtypes.QueryCodesRequest) (*wasmtypes.QueryCodesResponse, error) {
	resp, err := w.original.Codes(goCtx, req)
	if err == nil && resp != nil && len(resp.CodeInfos) > 0 {
		return resp, nil
	}
	if !isUserAPICall(goCtx) {
		return resp, err
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	rem, rerr := wq.Codes(qctx, req)
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.Codes(goCtx, req)
	}
	if rerr == nil && rem != nil && len(rem.CodeInfos) > 0 {
		for _, ci := range rem.CodeInfos {
			_ = w.app.materializeAndPinWasm(sdk.UnwrapSDKContext(goCtx), ci.CodeID)
		}
		return rem, nil
	}
	return resp, err
}

func (w *WasmQueryWrapper) PinnedCodes(goCtx context.Context, req *wasmtypes.QueryPinnedCodesRequest) (*wasmtypes.QueryPinnedCodesResponse, error) {
	resp, err := w.original.PinnedCodes(goCtx, req)
	if err == nil && resp != nil && len(resp.CodeIDs) > 0 {
		return resp, nil
	}
	if !isUserAPICall(goCtx) {
		return resp, err
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	rem, rerr := wq.PinnedCodes(qctx, req)
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.PinnedCodes(goCtx, req)
	}
	if rerr == nil && rem != nil && len(rem.CodeIDs) > 0 {
		for _, id := range rem.CodeIDs {
			_ = w.app.materializeAndPinWasm(sdk.UnwrapSDKContext(goCtx), id)
		}
		return rem, nil
	}
	return resp, err
}

func (w *WasmQueryWrapper) ContractInfo(goCtx context.Context, req *wasmtypes.QueryContractInfoRequest) (*wasmtypes.QueryContractInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	addr := sdk.MustAccAddressFromBech32(req.Address)
	if ci := w.keeper.GetContractInfo(ctx, addr); ci != nil {
		return &wasmtypes.QueryContractInfoResponse{Address: req.Address, ContractInfo: *ci}, nil
	}
	if !isUserAPICall(goCtx) {
		return nil, nil
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
		hostForTLS := normalized
		if h, _, e := net.SplitHostPort(normalized); e == nil {
			hostForTLS = h
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
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
	rem, rerr := wq.ContractInfo(qctx, &wasmtypes.QueryContractInfoRequest{Address: req.Address})
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		rem, rerr = wq.ContractInfo(goCtx, &wasmtypes.QueryContractInfoRequest{Address: req.Address})
	}
	if rerr != nil || rem == nil {
		return w.original.ContractInfo(goCtx, req)
	}
	_ = w.app.materializeAndPinWasm(ctx, rem.ContractInfo.CodeID)
	return rem, nil
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
