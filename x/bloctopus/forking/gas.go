package forking

import "github.com/cosmos/cosmos-sdk/store/types"

type sdkGasMeter struct {
	gm types.GasMeter
}

func NewSDKGasMeter(gm types.GasMeter) GasMeter {
	return &sdkGasMeter{gm: gm}
}

func (s *sdkGasMeter) ConsumeGas(amount uint64, descriptor string) {
	if s.gm == nil {
		return
	}
	s.gm.ConsumeGas(amount, descriptor)
}

func (s *sdkGasMeter) GasConsumed() uint64 {
	if s.gm == nil {
		return 0
	}
	return s.gm.GasConsumed()
}
