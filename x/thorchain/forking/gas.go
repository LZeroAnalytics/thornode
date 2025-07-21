package forking

import (
	storetypes "cosmossdk.io/store/types"
)

type SDKGasMeter struct {
	gasMeter storetypes.GasMeter
}

func NewSDKGasMeter(gasMeter storetypes.GasMeter) GasMeter {
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
