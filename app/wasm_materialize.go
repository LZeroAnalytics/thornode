package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"google.golang.org/grpc"
	"crypto/tls"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func (app *THORChainApp) materializeAndPinWasm(ctx sdk.Context, codeID uint64) error {
	fmt.Printf("[materialize] start codeID=%d forkHeight=%d wasmDir=%s\n", codeID, app.forkHeight, app.wasmDir)

	if codeID == 0 {
		return nil
	}

	var codeHash []byte
	var remoteCreator string
	var remoteInst *wasmtypes.AccessConfig
	if ci := app.WasmKeeper.GetCodeInfo(ctx, codeID); ci != nil && len(ci.CodeHash) > 0 {
		codeHash = ci.CodeHash
	}

	bz, err := app.WasmKeeper.GetByteCode(ctx, codeID)
	if err != nil || len(bz) == 0 || len(codeHash) == 0 {
		target := strings.TrimSpace(app.forkGRPC)
		if target == "" {
			target = "grpc.thor.pfc.zone:443"
		}
		useTLS := false
		normalized := target
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
			hostForTLS := normalized
			if h, _, e := net.SplitHostPort(normalized); e == nil {
				hostForTLS = h
			}
			tlsCfg := &tls.Config{
				ServerName: hostForTLS,
				MinVersion: tls.VersionTLS12,
			}
			dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
		} else {
			dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
		}
		conn, derr := grpc.Dial(normalized, dialOpt)
		if derr == nil {
			wq := wasmtypes.NewQueryClient(conn)
			md := metadata.New(nil)
			if app.forkHeight > 0 {
				md.Set("x-cosmos-block-height", fmt.Sprintf("%d", app.forkHeight))
			}
			qctx := metadata.NewOutgoingContext(context.Background(), md)
			resp, qerr := wq.Code(qctx, &wasmtypes.QueryCodeRequest{CodeId: codeID})
			needRetry := false
			if qerr != nil {
				msg := strings.ToLower(qerr.Error())
				if strings.Contains(msg, "invalid height") || strings.Contains(msg, "version mismatch") || strings.Contains(msg, "pruned") {
					needRetry = true
				}
			}
			if resp == nil || len(resp.Data) == 0 {
				needRetry = true
			}
			if needRetry {
				resp2, qerr2 := wq.Code(context.Background(), &wasmtypes.QueryCodeRequest{CodeId: codeID})
				if qerr2 == nil && resp2 != nil {
					resp = resp2
					qerr = nil
				}
			}
			if qerr == nil && resp != nil {
				if len(resp.Data) > 0 && len(bz) == 0 {
					bz = resp.Data
				}
				if len(resp.DataHash) > 0 && len(codeHash) == 0 {
					codeHash = resp.DataHash
				}
				if resp.Creator != "" && remoteCreator == "" {
					remoteCreator = resp.Creator
				}
				if resp.InstantiatePermission != nil && remoteInst == nil {
					remoteInst = resp.InstantiatePermission
				}
			}
			_ = conn.Close()
		}
	}

	if len(bz) == 0 {
		fmt.Printf("[materialize] no bytecode available for codeID=%d\n", codeID)
		return nil
	}

	sum := sha256.Sum256(bz)
	shaFilename := hex.EncodeToString(sum[:]) + ".wasm"
	var hashFilename string
	if len(codeHash) > 0 {
		hashFilename = hex.EncodeToString(codeHash) + ".wasm"
	}

	targets := []string{}
	if hashFilename != "" {
		targets = append(targets,
			filepath.Join(app.wasmDir, "wasm", "wasm", hashFilename),
			filepath.Join(app.wasmDir, "wasm", hashFilename),
		)
	}
	targets = append(targets,
		filepath.Join(app.wasmDir, "wasm", "wasm", shaFilename),
		filepath.Join(app.wasmDir, "wasm", shaFilename),
	)

	for _, p := range targets {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
	store := ctx.KVStore(app.GetKey(wasmtypes.StoreKey))
	codeKey := make([]byte, 1+8)
	codeKey[0] = 0x01
	binary.BigEndian.PutUint64(codeKey[1:], codeID)
	if existing := store.Get(codeKey); existing == nil {
		ci := wasmtypes.CodeInfo{
			CodeHash:          codeHash,
			Creator:           remoteCreator,
			InstantiateConfig: remoteInst,
		}
		if b, merr := app.appCodec.Marshal(&ci); merr == nil {
			store.Set(codeKey, b)
		}
	}

			return err
		}
		if _, err := os.Stat(p); err != nil {
			fmt.Printf("[materialize] writing wasm file: %s\n", p)
			if writeErr := os.WriteFile(p, bz, 0o644); writeErr != nil {
				return writeErr
			}
		}
	}

	pk := wasmkeeper.NewGovPermissionKeeper(app.WasmKeeper)
	_ = pk.UnpinCode(ctx, codeID)
	fmt.Printf("[materialize] pin codeID=%d\n", codeID)
	if err := pk.PinCode(ctx, codeID); err != nil {
		return err
	}
	return nil
}

func (app *THORChainApp) materializeAndPinWasmGO(ctx context.Context, codeID uint64) error {
	return app.materializeAndPinWasm(sdk.UnwrapSDKContext(ctx), codeID)
}
