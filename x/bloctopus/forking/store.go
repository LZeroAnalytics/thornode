package forking

import (
	"context"

	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/store/prefix"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type KVStore struct {
	ctx      context.Context
	store    storetypes.KVStore
	endpoint string

	metaPopulated bool
}

func NewKVStore(ctx context.Context, store storetypes.KVStore, endpoint string) *KVStore {
	return &KVStore{ctx: ctx, store: store, endpoint: endpoint}
}

func (k *KVStore) ensureDenomMetadata() {
	if k.metaPopulated {
		return
	}
	rc, err := NewRemoteClient(k.endpoint)
	if err != nil {
		return
	}
	defer rc.Close()
	resp, err := rc.DenomsMetadata(k.ctx)
	if err != nil || resp == nil || len(resp.Metadatas) == 0 {
		return
	}
	p := prefix.NewStore(k.store, banktypes.DenomMetadataPrefix)
	for _, m := range resp.Metadatas {
		key := []byte(m.Base)
		bz, _ := banktypes.ModuleCdc().Marshal(&m)
		p.Set(key, bz)
	}
	k.metaPopulated = true
}

func (k *KVStore) Get(key []byte) []byte {
	if len(key) > 0 && key[0] == banktypes.DenomMetadataPrefix[0] {
		if v := k.store.Get(key); v == nil {
			k.ensureDenomMetadata()
			return k.store.Get(key)
		}
	}
	return k.store.Get(key)
}

func (k *KVStore) Has(key []byte) bool {
	if len(key) > 0 && key[0] == banktypes.DenomMetadataPrefix[0] {
		if !k.store.Has(key) {
			k.ensureDenomMetadata()
			return k.store.Has(key)
		}
	}
	return k.store.Has(key)
}

func (k *KVStore) Set(key, value []byte) {
	k.store.Set(key, value)
}

func (k *KVStore) Delete(key []byte) {
	k.store.Delete(key)
}

func (k *KVStore) Iterator(start, end []byte) storetypes.Iterator {
	if len(start) > 0 && start[0] == banktypes.DenomMetadataPrefix[0] {
		k.ensureDenomMetadata()
	}
	return k.store.Iterator(start, end)
}

func (k *KVStore) ReverseIterator(start, end []byte) storetypes.Iterator {
	if len(start) > 0 && start[0] == banktypes.DenomMetadataPrefix[0] {
		k.ensureDenomMetadata()
	}
	return k.store.ReverseIterator(start, end)
}
