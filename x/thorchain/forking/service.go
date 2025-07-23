package forking

import (
	"context"
	"sync"

	storetypes "cosmossdk.io/core/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type forkingKVStoreService struct {
	parent       storetypes.KVStoreService
	remoteClient RemoteClient
	cache        Cache
	config       RemoteConfig
	storeKey     string

	mu              sync.RWMutex
	pinnedHeight    int64
	genesisMode     bool
	blockProcessing bool

	stats ForkingStats
}

func NewForkingKVStoreService(
	parent storetypes.KVStoreService,
	remoteClient RemoteClient,
	cache Cache,
	config RemoteConfig,
	storeKey string,
) ForkingKVStoreService {
	return &forkingKVStoreService{
		parent:       parent,
		remoteClient: remoteClient,
		cache:        cache,
		config:       config,
		storeKey:     storeKey,
		pinnedHeight: config.ForkHeight,
		genesisMode:  true,
		stats:        ForkingStats{},
	}
}

func (f *forkingKVStoreService) OpenKVStore(ctx context.Context) storetypes.KVStore {
	parentStore := f.parent.OpenKVStore(ctx)

	var gasMeter GasMeter
	var sdkCtx *sdk.Context
	if sdkContext, ok := ctx.(sdk.Context); ok {
		gasMeter = NewSDKGasMeter(sdkContext.GasMeter())
		sdkCtx = &sdkContext
	}

	return NewForkingKVStore(parentStore, f.remoteClient, f.cache, f.config, f.storeKey, f, gasMeter, sdkCtx)
}

func (f *forkingKVStoreService) GetStats() ForkingStats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stats
}

func (f *forkingKVStoreService) BeginBlock(height int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if height > 1 {
		f.genesisMode = false
	}
	f.blockProcessing = true

	return nil
}

func (f *forkingKVStoreService) EndBlock() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blockProcessing = false
	return nil
}

func (f *forkingKVStoreService) GetPinnedHeight() int64 {
	return f.pinnedHeight
}

func (f *forkingKVStoreService) IsGenesisMode() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.genesisMode
}

func (f *forkingKVStoreService) IsBlockProcessing() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.blockProcessing
}

func (f *forkingKVStoreService) updateStats(remoteFetch bool, cacheHit bool, gasUsed uint64, proofFailed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if remoteFetch {
		f.stats.RemoteFetches++
		f.stats.GasConsumed += gasUsed
	}

	if proofFailed {
		f.stats.ProofFailures++
	}

	alpha := 0.1 // smoothing factor
	if cacheHit {
		f.stats.CacheHitRatio = alpha*1.0 + (1-alpha)*f.stats.CacheHitRatio
	} else {
		f.stats.CacheHitRatio = alpha*0.0 + (1-alpha)*f.stats.CacheHitRatio
	}
}
