package app

import (
	"context"

	"github.com/cosmos/cosmos-sdk/baseapp"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/constants"
)

var _ grpc.ServiceRegistrar = &QueryServiceRouter{}

type QueryServiceRouter struct {
	*baseapp.GRPCQueryRouter
	customRoutes map[string]interface{}
}

func NewQueryServiceRouter(bAppQsr *baseapp.GRPCQueryRouter) *QueryServiceRouter {
	return &QueryServiceRouter{
		GRPCQueryRouter: bAppQsr,
		customRoutes:    make(map[string]interface{}),
	}
}

func (qsr *QueryServiceRouter) RegisterService(sd *grpc.ServiceDesc, handler interface{}) {
	if customHandler := qsr.customRoutes[sd.ServiceName]; customHandler != nil {
		handler = customHandler
	}

	qsr.GRPCQueryRouter.RegisterService(sd, handler)
}

func (qsr *QueryServiceRouter) AddCustomRoute(serviceName string, handler interface{}) {
	qsr.customRoutes[serviceName] = handler
}

type BankQueryWrapper struct {
	originalHandler banktypes.QueryServer
}

func NewBankQueryWrapper(originalHandler banktypes.QueryServer) *BankQueryWrapper {
	return &BankQueryWrapper{originalHandler: originalHandler}
}

func (w *BankQueryWrapper) extractUserAPICall(goCtx context.Context) context.Context {
	ctx := sdk.UnwrapSDKContext(goCtx)
	
	if md, ok := metadata.FromIncomingContext(goCtx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxUserAPICall, true))
		}
	}
	
	return ctx
}

func (w *BankQueryWrapper) Balance(goCtx context.Context, req *banktypes.QueryBalanceRequest) (*banktypes.QueryBalanceResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.Balance(ctx, req)
}

func (w *BankQueryWrapper) AllBalances(goCtx context.Context, req *banktypes.QueryAllBalancesRequest) (*banktypes.QueryAllBalancesResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.AllBalances(ctx, req)
}

func (w *BankQueryWrapper) SpendableBalances(goCtx context.Context, req *banktypes.QuerySpendableBalancesRequest) (*banktypes.QuerySpendableBalancesResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.SpendableBalances(ctx, req)
}

func (w *BankQueryWrapper) SpendableBalanceByDenom(goCtx context.Context, req *banktypes.QuerySpendableBalanceByDenomRequest) (*banktypes.QuerySpendableBalanceByDenomResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.SpendableBalanceByDenom(ctx, req)
}

func (w *BankQueryWrapper) TotalSupply(goCtx context.Context, req *banktypes.QueryTotalSupplyRequest) (*banktypes.QueryTotalSupplyResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.TotalSupply(ctx, req)
}

func (w *BankQueryWrapper) SupplyOf(goCtx context.Context, req *banktypes.QuerySupplyOfRequest) (*banktypes.QuerySupplyOfResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.SupplyOf(ctx, req)
}

func (w *BankQueryWrapper) Params(goCtx context.Context, req *banktypes.QueryParamsRequest) (*banktypes.QueryParamsResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.Params(ctx, req)
}

func (w *BankQueryWrapper) DenomMetadata(goCtx context.Context, req *banktypes.QueryDenomMetadataRequest) (*banktypes.QueryDenomMetadataResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.DenomMetadata(ctx, req)
}

func (w *BankQueryWrapper) DenomsMetadata(goCtx context.Context, req *banktypes.QueryDenomsMetadataRequest) (*banktypes.QueryDenomsMetadataResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.DenomsMetadata(ctx, req)
}

func (w *BankQueryWrapper) DenomOwners(goCtx context.Context, req *banktypes.QueryDenomOwnersRequest) (*banktypes.QueryDenomOwnersResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.DenomOwners(ctx, req)
}

func (w *BankQueryWrapper) SendEnabled(goCtx context.Context, req *banktypes.QuerySendEnabledRequest) (*banktypes.QuerySendEnabledResponse, error) {
	ctx := w.extractUserAPICall(goCtx)
	return w.originalHandler.SendEnabled(ctx, req)
}
