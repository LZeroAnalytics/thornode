package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
)

func (app *THORChainApp) materializeAndPinWasm(ctx sdk.Context, codeID uint64) error {
	if codeID == 0 {
		return nil
	}

	bz, err := app.WasmKeeper.GetByteCode(ctx, codeID)
	if err != nil || len(bz) == 0 {
		return err
	}

	sum := sha256.Sum256(bz)
	filename := hex.EncodeToString(sum[:]) + ".wasm"
	target := filepath.Join(app.wasmDir, "wasm", "wasm", filename)

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(target); err != nil {
		if writeErr := os.WriteFile(target, bz, 0o644); writeErr != nil {
			return writeErr
		}
	}

	_ = app.WasmKeeper.UnpinCode(ctx, codeID)
	if pinErr := app.WasmKeeper.PinCode(ctx, codeID); pinErr != nil {
		return pinErr
	}
	return nil
}

func (app *THORChainApp) materializeAndPinWasmGO(ctx context.Context, codeID uint64) error {
	return app.materializeAndPinWasm(sdk.UnwrapSDKContext(ctx), codeID)
}
