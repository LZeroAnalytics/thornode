package forking

import (
	"context"
	"time"

	storetypes "cosmossdk.io/core/store"
)

type RemoteConfig struct {
	GRPC            string
	ChainID         string
	ForkHeight      int64
	TrustHeight     int64
	TrustHash       string
	TrustingPeriod  time.Duration
	MaxClockDrift   time.Duration
	Timeout         time.Duration
	CacheEnabled    bool
	CacheSize       int
	GasCostPerFetch uint64
}

type RemoteClient interface {
	GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error)
	GetRange(ctx context.Context, storeKey string, start, end []byte, height int64) ([]KeyValue, error)
	GetLatestHeight(ctx context.Context) (int64, error)
	Close() error
}

type KeyValue struct {
	Key   []byte
	Value []byte
}

type Cache interface {
	Get(key []byte) []byte
	Set(key []byte, value []byte)
	Has(key []byte) (bool, error)
	Delete(key []byte)
	Clear()
}

type ForkingKVStoreService interface {
	storetypes.KVStoreService
	GetStats() ForkingStats
	BeginBlock(height int64) error
	EndBlock() error
	IsBlockProcessing() bool
}

type ForkingStats struct {
	RemoteFetches uint64
	CacheHitRatio float64
	GasConsumed   uint64
	ProofFailures uint64
}

type ForkingKVStore interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	Iterator(start, end []byte) (storetypes.Iterator, error)
	ReverseIterator(start, end []byte) (storetypes.Iterator, error)
	GetStats() ForkingStats
}

type GasMeter interface {
	ConsumeGas(amount uint64, descriptor string)
	GasConsumed() uint64
}
