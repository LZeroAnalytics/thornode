package keeper

import (
	"context"

	"google.golang.org/grpc/metadata"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

var _ types.QueryServer = Keeper{}

func (k Keeper) DenomAdmin(ctx context.Context, req *types.QueryDenomAdminRequest) (*types.QueryDenomAdminResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userFlag := md.Get("user-api-call"); len(userFlag) > 0 && userFlag[0] == "true" {
			sdkCtx = sdkCtx.WithContext(context.WithValue(sdkCtx.Context(), constants.CtxUserAPICall, true))
		}
	}

	admin, err := k.GetAdmin(sdkCtx, req.GetDenom())
	if err != nil {
		return nil, err
	}

	return &types.QueryDenomAdminResponse{Admin: admin.String()}, nil
}
