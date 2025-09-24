package bloctopus

import (
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

type WasmMgrPermissionless struct {
	keeper     keeper.Keeper
	wasmKeeper wasmkeeper.Keeper
}

func NewWasmMgrPermissionless(k keeper.Keeper, wk wasmkeeper.Keeper) (*WasmMgrPermissionless, error) {
	return &WasmMgrPermissionless{
		keeper:     k,
		wasmKeeper: wk,
	}, nil
}

func (m WasmMgrPermissionless) permKeeper() *wasmkeeper.PermissionedKeeper {
	return wasmkeeper.NewDefaultPermissionKeeper(m.wasmKeeper)
}

func (m WasmMgrPermissionless) StoreCode(
	ctx cosmos.Context,
	creator sdk.AccAddress,
	wasmCode []byte,
) (codeID uint64, checksum []byte, err error) {
	return m.permKeeper().Create(ctx, creator, wasmCode, nil)
}

func (m WasmMgrPermissionless) InstantiateContract(
	ctx cosmos.Context,
	codeID uint64,
	creator, admin sdk.AccAddress,
	initMsg []byte,
	label string,
	deposit sdk.Coins,
) (sdk.AccAddress, []byte, error) {
	if err := m.maybePin(ctx, codeID); err != nil {
		return nil, nil, err
	}
	return m.permKeeper().Instantiate(ctx, codeID, creator, admin, initMsg, label, deposit)
}

func (m WasmMgrPermissionless) InstantiateContract2(
	ctx cosmos.Context,
	codeID uint64,
	creator, admin sdk.AccAddress,
	initMsg []byte,
	label string,
	deposit sdk.Coins,
	salt []byte,
	fixMsg bool,
) (sdk.AccAddress, []byte, error) {
	if err := m.maybePin(ctx, codeID); err != nil {
		return nil, nil, err
	}
	return m.permKeeper().Instantiate2(ctx, codeID, creator, admin, initMsg, label, deposit, salt, fixMsg)
}

func (m WasmMgrPermissionless) ExecuteContract(
	ctx cosmos.Context,
	contractAddr, senderAddr sdk.AccAddress,
	msg []byte,
	coins sdk.Coins,
) ([]byte, error) {
	return m.permKeeper().Execute(ctx, contractAddr, senderAddr, msg, coins)
}

func (m WasmMgrPermissionless) MigrateContract(
	ctx cosmos.Context,
	contractAddress, caller sdk.AccAddress,
	newCodeID uint64,
	msg []byte,
) ([]byte, error) {
	if err := m.maybePin(ctx, newCodeID); err != nil {
		return nil, err
	}
	data, err := m.permKeeper().Migrate(ctx, contractAddress, caller, newCodeID, msg)
	if err != nil {
		return nil, err
	}
	contractInfo := m.wasmKeeper.GetContractInfo(ctx, contractAddress)
	if contractInfo != nil {
		if err := m.maybeUnpin(ctx, contractInfo.CodeID); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (m WasmMgrPermissionless) SudoContract(
	ctx cosmos.Context,
	contractAddress, _ sdk.AccAddress,
	msg []byte,
) ([]byte, error) {
	return m.permKeeper().Sudo(ctx, contractAddress, msg)
}

func (m WasmMgrPermissionless) UpdateAdmin(
	ctx cosmos.Context,
	contractAddress, sender, newAdmin sdk.AccAddress,
) ([]byte, error) {
	return nil, m.permKeeper().UpdateContractAdmin(ctx, contractAddress, sender, newAdmin)
}

func (m WasmMgrPermissionless) ClearAdmin(
	ctx cosmos.Context,
	contractAddress, sender sdk.AccAddress,
) ([]byte, error) {
	return nil, m.permKeeper().ClearContractAdmin(ctx, contractAddress, sender)
}

func (m WasmMgrPermissionless) getCodeInfo(ctx cosmos.Context, id uint64) (*wasmtypes.CodeInfo, error) {
	codeInfo := m.wasmKeeper.GetCodeInfo(ctx, id)
	if codeInfo == nil {
		return nil, wasmtypes.ErrNotFound
	}
	return codeInfo, nil
}

func (m WasmMgrPermissionless) maybePin(ctx cosmos.Context, codeId uint64) error {
	var instanceCount int
	m.wasmKeeper.IterateContractsByCode(ctx, codeId, func(address sdk.AccAddress) bool {
		instanceCount++
		return true
	})
	if instanceCount == 0 {
		if err := m.permKeeper().PinCode(ctx, codeId); err != nil {
			return err
		}
	}
	return nil
}

func (m WasmMgrPermissionless) maybeUnpin(ctx cosmos.Context, codeId uint64) error {
	var instanceCount int
	m.wasmKeeper.IterateContractsByCode(ctx, codeId, func(address sdk.AccAddress) bool {
		instanceCount++
		return true
	})
	if instanceCount == 0 {
		if err := m.permKeeper().UnpinCode(ctx, codeId); err != nil {
			return err
		}
	}
	return nil
}
