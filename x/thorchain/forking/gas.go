package forking

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type SDKGasMeter struct {
	gasMeter sdk.GasMeter
}

func NewSDKGasMeter(gasMeter sdk.GasMeter) GasMeter {
	if gasMeter == nil {
		return nil
	}
	return &SDKGasMeter{gasMeter: gasMeter}
}

func (g *SDKGasMeter) ConsumeGas(amount uint64, descriptor string) {
	g.gasMeter.ConsumeGas(amount, descriptor)
}

func (g *SDKGasMeter) GasConsumed() uint64 {
	return g.gasMeter.GasConsumed()
}
