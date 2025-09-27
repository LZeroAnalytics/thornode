package forking

import storetypes "cosmossdk.io/store/types"

type MergedIterator struct {
	local storetypes.Iterator
}

func (it *MergedIterator) Close() error {
	if it.local != nil {
		return it.local.Close()
	}
	return nil
}
