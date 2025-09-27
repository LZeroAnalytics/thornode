package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func mergeCoins(local sdk.Coins, remote sdk.Coins) sdk.Coins {
	out := sdk.NewCoins(local...)
	for _, rc := range remote {
		if out.AmountOfNoDenomValidation(rc.Denom).IsZero() && !rc.Amount.IsZero() {
			out = out.Add(rc)
		}
	}
	return out.Sort()
}

func mergeMetadata(local []banktypes.Metadata, remote []banktypes.Metadata) []banktypes.Metadata {
	out := make([]banktypes.Metadata, 0, len(local)+len(remote))
	seen := make(map[string]struct{}, len(local))
	for _, m := range local {
		out = append(out, m)
		seen[m.Base] = struct{}{}
	}
	for _, m := range remote {
		if _, ok := seen[m.Base]; ok {
			continue
		}
		out = append(out, m)
	}
	return out
}
