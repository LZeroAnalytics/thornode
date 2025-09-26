package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

func mergeCoins(local sdk.Coins, remote sdk.Coins) sdk.Coins {
	out := sdk.NewCoins(local...)
	for _, rc := range remote {
		if out.AmountOfNoDenomValidation(rc.Denom).IsZero() && !rc.Amount.IsZero() {
			out = out.Add(rc)
		}
	}
	return out.Sort()
}
