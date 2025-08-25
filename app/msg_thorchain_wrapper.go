package app

import (
	"context"
	"fmt"
	"net"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	cwtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	thtypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type ThorchainMsgWrapper struct {
	thtypes.UnimplementedMsgServer
	app      *THORChainApp
	keeper   *wasmkeeper.Keeper
	original thtypes.MsgServer
}

func NewThorchainMsgWrapper(app *THORChainApp, k *wasmkeeper.Keeper, original thtypes.MsgServer) *ThorchainMsgWrapper {
	return &ThorchainMsgWrapper{
		app:      app,
		keeper:   k,
		original: original,
	}
}

func (w *ThorchainMsgWrapper) withMaterialized(goCtx context.Context, codeID uint64) (context.Context, error) {
	if codeID != 0 {
		if err := w.app.materializeAndPinWasm(sdk.UnwrapSDKContext(goCtx), codeID); err != nil {
			return goCtx, err
		}
	}
	return goCtx, nil
}

func (w *ThorchainMsgWrapper) ensureMaterializedByAddress(ctx sdk.Context, bech32Addr string) {
	addr, err := sdk.AccAddressFromBech32(bech32Addr)
	if err == nil {
		if ci := w.keeper.GetContractInfo(ctx, addr); ci != nil {
			_ = w.app.materializeAndPinWasm(ctx, ci.CodeID)
			return
		}
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
		return
	}
	defer conn.Close()
	wq := cwtypes.NewQueryClient(conn)
	md := metadata.New(nil)
	if w.app.forkHeight > 0 {
		md.Set("x-cosmos-block-height", fmt.Sprintf("%d", w.app.forkHeight))
	}
	qctx := metadata.NewOutgoingContext(ctx.Context(), md)
	resp, rerr := wq.ContractInfo(qctx, &cwtypes.QueryContractInfoRequest{Address: bech32Addr})
	if rerr != nil && shouldRetryWithoutHeight(rerr) {
		resp, rerr = wq.ContractInfo(ctx.Context(), &cwtypes.QueryContractInfoRequest{Address: bech32Addr})
	}
	if rerr != nil || resp == nil || resp.ContractInfo.CodeID == 0 {
		return
	}
	_ = w.app.materializeAndPinWasm(ctx, resp.ContractInfo.CodeID)
}


func (w *ThorchainMsgWrapper) StoreCode(ctx context.Context, req *cwtypes.MsgStoreCode) (*cwtypes.MsgStoreCodeResponse, error) {
	return w.original.StoreCode(ctx, req)
}

func (w *ThorchainMsgWrapper) InstantiateContract(goCtx context.Context, req *cwtypes.MsgInstantiateContract) (*cwtypes.MsgInstantiateContractResponse, error) {
	if _, err := w.withMaterialized(goCtx, req.CodeID); err != nil {
	}
	return w.original.InstantiateContract(goCtx, req)
}

func (w *ThorchainMsgWrapper) InstantiateContract2(goCtx context.Context, req *cwtypes.MsgInstantiateContract2) (*cwtypes.MsgInstantiateContract2Response, error) {
	if _, err := w.withMaterialized(goCtx, req.CodeID); err != nil {
	}
	return w.original.InstantiateContract2(goCtx, req)
}

func (w *ThorchainMsgWrapper) ExecuteContract(goCtx context.Context, req *cwtypes.MsgExecuteContract) (*cwtypes.MsgExecuteContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	w.ensureMaterializedByAddress(ctx, req.Contract)
	return w.original.ExecuteContract(goCtx, req)
}

func (w *ThorchainMsgWrapper) MigrateContract(goCtx context.Context, req *cwtypes.MsgMigrateContract) (*cwtypes.MsgMigrateContractResponse, error) {
	if _, err := w.withMaterialized(goCtx, req.CodeID); err != nil {
	}
	return w.original.MigrateContract(goCtx, req)
}

func (w *ThorchainMsgWrapper) SudoContract(ctx context.Context, req *cwtypes.MsgSudoContract) (*cwtypes.MsgSudoContractResponse, error) {
	return w.original.SudoContract(ctx, req)
}

func (w *ThorchainMsgWrapper) UpdateAdmin(ctx context.Context, req *cwtypes.MsgUpdateAdmin) (*cwtypes.MsgUpdateAdminResponse, error) {
	return w.original.UpdateAdmin(ctx, req)
}

func (w *ThorchainMsgWrapper) ClearAdmin(ctx context.Context, req *cwtypes.MsgClearAdmin) (*cwtypes.MsgClearAdminResponse, error) {
	return w.original.ClearAdmin(ctx, req)
}


func (w *ThorchainMsgWrapper) Ban(ctx context.Context, msg *thtypes.MsgBan) (*thtypes.MsgEmpty, error) {
	return w.original.Ban(ctx, msg)
}
func (w *ThorchainMsgWrapper) Deposit(ctx context.Context, msg *thtypes.MsgDeposit) (*thtypes.MsgEmpty, error) {
	return w.original.Deposit(ctx, msg)
}
func (w *ThorchainMsgWrapper) ErrataTx(ctx context.Context, msg *thtypes.MsgErrataTx) (*thtypes.MsgEmpty, error) {
	return w.original.ErrataTx(ctx, msg)
}
func (w *ThorchainMsgWrapper) ErrataTxQuorum(ctx context.Context, msg *thtypes.MsgErrataTxQuorum) (*thtypes.MsgEmpty, error) {
	return w.original.ErrataTxQuorum(ctx, msg)
}
func (w *ThorchainMsgWrapper) Mimir(ctx context.Context, msg *thtypes.MsgMimir) (*thtypes.MsgEmpty, error) {
	return w.original.Mimir(ctx, msg)
}
func (w *ThorchainMsgWrapper) ModifyLimitSwap(ctx context.Context, msg *thtypes.MsgModifyLimitSwap) (*thtypes.MsgEmpty, error) {
	return w.original.ModifyLimitSwap(ctx, msg)
}
func (w *ThorchainMsgWrapper) NetworkFee(ctx context.Context, msg *thtypes.MsgNetworkFee) (*thtypes.MsgEmpty, error) {
	return w.original.NetworkFee(ctx, msg)
}
func (w *ThorchainMsgWrapper) NetworkFeeQuorum(ctx context.Context, msg *thtypes.MsgNetworkFeeQuorum) (*thtypes.MsgEmpty, error) {
	return w.original.NetworkFeeQuorum(ctx, msg)
}
func (w *ThorchainMsgWrapper) NodePauseChain(ctx context.Context, msg *thtypes.MsgNodePauseChain) (*thtypes.MsgEmpty, error) {
	return w.original.NodePauseChain(ctx, msg)
}
func (w *ThorchainMsgWrapper) ObservedTxIn(ctx context.Context, msg *thtypes.MsgObservedTxIn) (*thtypes.MsgEmpty, error) {
	return w.original.ObservedTxIn(ctx, msg)
}
func (w *ThorchainMsgWrapper) ObservedTxOut(ctx context.Context, msg *thtypes.MsgObservedTxOut) (*thtypes.MsgEmpty, error) {
	return w.original.ObservedTxOut(ctx, msg)
}
func (w *ThorchainMsgWrapper) ObservedTxQuorum(ctx context.Context, msg *thtypes.MsgObservedTxQuorum) (*thtypes.MsgEmpty, error) {
	return w.original.ObservedTxQuorum(ctx, msg)
}
func (w *ThorchainMsgWrapper) ThorSend(ctx context.Context, msg *thtypes.MsgSend) (*thtypes.MsgEmpty, error) {
	return w.original.ThorSend(ctx, msg)
}
func (w *ThorchainMsgWrapper) SetIPAddress(ctx context.Context, msg *thtypes.MsgSetIPAddress) (*thtypes.MsgEmpty, error) {
	return w.original.SetIPAddress(ctx, msg)
}
func (w *ThorchainMsgWrapper) SetNodeKeys(ctx context.Context, msg *thtypes.MsgSetNodeKeys) (*thtypes.MsgEmpty, error) {
	return w.original.SetNodeKeys(ctx, msg)
}
func (w *ThorchainMsgWrapper) Solvency(ctx context.Context, msg *thtypes.MsgSolvency) (*thtypes.MsgEmpty, error) {
	return w.original.Solvency(ctx, msg)
}
func (w *ThorchainMsgWrapper) SolvencyQuorum(ctx context.Context, msg *thtypes.MsgSolvencyQuorum) (*thtypes.MsgEmpty, error) {
	return w.original.SolvencyQuorum(ctx, msg)
}
func (w *ThorchainMsgWrapper) TssKeysignFail(ctx context.Context, msg *thtypes.MsgTssKeysignFail) (*thtypes.MsgEmpty, error) {
	return w.original.TssKeysignFail(ctx, msg)
}
func (w *ThorchainMsgWrapper) TssPool(ctx context.Context, msg *thtypes.MsgTssPool) (*thtypes.MsgEmpty, error) {
	return w.original.TssPool(ctx, msg)
}
func (w *ThorchainMsgWrapper) SetVersion(ctx context.Context, msg *thtypes.MsgSetVersion) (*thtypes.MsgEmpty, error) {
	return w.original.SetVersion(ctx, msg)
}
func (w *ThorchainMsgWrapper) ProposeUpgrade(ctx context.Context, msg *thtypes.MsgProposeUpgrade) (*thtypes.MsgEmpty, error) {
	return w.original.ProposeUpgrade(ctx, msg)
}
func (w *ThorchainMsgWrapper) ApproveUpgrade(ctx context.Context, msg *thtypes.MsgApproveUpgrade) (*thtypes.MsgEmpty, error) {
	return w.original.ApproveUpgrade(ctx, msg)
}
func (w *ThorchainMsgWrapper) RejectUpgrade(ctx context.Context, msg *thtypes.MsgRejectUpgrade) (*thtypes.MsgEmpty, error) {
	return w.original.RejectUpgrade(ctx, msg)
}
