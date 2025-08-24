package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (app *THORChainApp) materializeAndPinWasm(ctx sdk.Context, codeID uint64) error {
	if codeID == 0 {
		return nil
	}

	bz, err := app.WasmKeeper.GetByteCode(ctx, codeID)
	if err != nil || len(bz) == 0 {
		target := "grpc.thor.pfc.zone:443"
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
		if derr == nil {
			wq := wasmtypes.NewQueryClient(conn)
			resp, qerr := wq.Code(ctx.Context(), &wasmtypes.QueryCodeRequest{CodeId: codeID})
			if qerr == nil && resp != nil && len(resp.Data) > 0 {
				bz = resp.Data
			}
			_ = conn.Close()
		}
	}

	if len(bz) == 0 {
		return nil
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

	pk := wasmkeeper.NewGovPermissionKeeper(app.WasmKeeper)
	_ = pk.UnpinCode(ctx, codeID)
	if err := pk.PinCode(ctx, codeID); err != nil {
		return err
	}
	return nil
}

func (app *THORChainApp) materializeAndPinWasmGO(ctx context.Context, codeID uint64) error {
	return app.materializeAndPinWasm(sdk.UnwrapSDKContext(ctx), codeID)
}
