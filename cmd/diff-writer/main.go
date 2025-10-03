package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cosmossdk.io/store/streaming/abci"
	storetypes "cosmossdk.io/store/types"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/hashicorp/go-plugin"
)

const (
	envOutDir     = "THOR_DIFF_OUT"
	envBaseHeight = "THOR_DIFF_BASE_HEIGHT"
	defaultOutDir = "/root/.thornode/diffs"
)

type filePlugin struct {
	outDir     string
	baseHeight int64
	lastHeight int64
}

type kvWrite struct {
	Store string `json:"store"`
	Op    string `json:"op"`
	Key   string `json:"key_hex"`
	Value string `json:"value_b64,omitempty"`
}

type diffFile struct {
	BaseHeight  int64     `json:"base_height"`
	Height      int64     `json:"height"`
	Time        string    `json:"time"`
	StoreWrites []kvWrite `json:"store_writes"`
}

func (p *filePlugin) ListenFinalizeBlock(ctx context.Context, req abcitypes.RequestFinalizeBlock, res abcitypes.ResponseFinalizeBlock) error {
	p.lastHeight = req.Height
	return nil
}

func (p *filePlugin) ListenCommit(ctx context.Context, res abcitypes.ResponseCommit, changeSet []*storetypes.StoreKVPair) error {
	height := p.lastHeight
	if height == 0 {
		height = res.RetainHeight
	}
	if height == 0 {
		height = time.Now().UnixNano()
	}
	df := diffFile{
		BaseHeight:  p.baseHeight,
		Height:      height,
		Time:        time.Now().UTC().Format(time.RFC3339Nano),
		StoreWrites: make([]kvWrite, 0, len(changeSet)),
	}
	for _, ch := range changeSet {
		op := "set"
		if ch.Delete {
			op = "delete"
		}
		df.StoreWrites = append(df.StoreWrites, kvWrite{
			Store: ch.StoreKey,
			Op:    op,
			Key:   hex.EncodeToString(ch.Key),
			Value: func() string {
				if ch.Delete {
					return ""
				}
				return base64.StdEncoding.EncodeToString(ch.Value)
			}(),
		})
	}
	if err := os.MkdirAll(p.outDir, 0o755); err != nil {
		return err
	}
	out := filepath.Join(p.outDir, strconv.FormatInt(df.Height, 10)+".json")
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(&df)
}

func main() {
	outDir := os.Getenv(envOutDir)
	if outDir == "" {
		outDir = defaultOutDir
	}
	var baseHeight int64
	if bh := os.Getenv(envBaseHeight); bh != "" {
		if v, err := strconv.ParseInt(bh, 10, 64); err == nil {
			baseHeight = v
		}
	}
	impl := &filePlugin{outDir: outDir, baseHeight: baseHeight}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: abci.Handshake,
		Plugins: map[string]plugin.Plugin{
			"abci":    &abci.ListenerGRPCPlugin{Impl: impl},
			"abci_v1": &abci.ListenerGRPCPlugin{Impl: impl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
