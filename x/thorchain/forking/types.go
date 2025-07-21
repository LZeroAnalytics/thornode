package forking

import (
	"context"
	"time"

	storetypes "cosmossdk.io/core/store"
)

type RemoteConfig struct {
	RPC string
	ChainID string
	TrustHeight int64
	TrustHash string
	TrustingPeriod time.Duration
	MaxClockDrift time.Duration
	Timeout time.Duration
	CacheEnabled bool
	CacheSize int
	GasCostPerFetch uint64
}

func DefaultRemoteConfig() RemoteConfig {
	return RemoteConfig{
		RPC:            "https://thornode.ninerealms.com:26657",
		ChainID:        "thorchain-mainnet-v1",
		TrustHeight:    0, // Will be set dynamically
		TrustHash:      "", // Will be set dynamically
		TrustingPeriod: 24 * time.Hour,
		MaxClockDrift:  10 * time.Second,
		Timeout:        30 * time.Second,
		CacheEnabled:   true,
		CacheSize:      10000,
		GasCostPerFetch: 1000,
	}
}

type RemoteClient interface {
	GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error)
	GetLatestHeight(ctx context.Context) (int64, error)
	Close() error
}

type Cache interface {
	Get(key []byte) []byte
	Set(key []byte, value []byte)
	Has(key []byte) bool
	Delete(key []byte)
	Clear()
}

type ForkingKVStoreService interface {
	storetypes.KVStoreService
	SetRemoteHeight(height int64)
	GetRemoteHeight() int64
	GetStats() ForkingStats
	BeginBlock(height int64) error
	EndBlock() error
}

type ForkingStats struct {
	RemoteFetches uint64
	CacheHitRatio float64
	GasConsumed uint64
	ProofFailures uint64
}

type ForkingKVStore interface {
	storetypes.KVStore
	GetStats() ForkingStats
}

type GasMeter interface {
	ConsumeGas(amount uint64, descriptor string)
	GasConsumed() uint64
}
