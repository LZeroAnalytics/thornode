# THORChain Forking Module

This module implements forking capabilities for THORNode, allowing a local node to pull contract and account data in real-time from a remote THORChain mainnet when they don't exist in the local database.

## Overview

The forking module works by wrapping the standard Cosmos SDK `KVStoreService` with a `ForkingKVStoreService` that:

1. First checks the local store for requested data
2. If not found locally, checks an in-memory LRU cache
3. If not cached, fetches from remote THORChain gRPC with data verification
4. Caches and stores the result locally for future access

## Architecture

```
┌─────────────────────────────────┐
│   THORChain App                 │
│  ─────────────────────────────  │
│  Modules / Keepers              │  ← transaction executes
│        │                        │
│  ForkingKVStoreService          │  ← wraps standard KVStoreService
│        │                        │
│  ForkingKVStore                 │  ← wraps individual KVStore
│        │                        │
│  ┌─────▼─────┐ ┌─────────────┐  │
│  │Local Store│ │   Cache     │  │  1. local Get()
│  └───────────┘ └─────────────┘  │  2. cache check
│        │                        │  3. ⇒ remoteFetch()
│        ▼                        │
│  Remote gRPC (mainnet)          │
│    + Tendermint Light Client    │
└─────────────────────────────────┘
```

## Components

### ForkingKVStoreService
- Wraps the standard `storetypes.KVStoreService`
- Manages remote height for deterministic queries
- Tracks statistics (cache hits, remote fetches, gas usage)

### ForkingKVStore
- Wraps individual `storetypes.KVStore` instances
- Implements the forking logic in the `Get()` method
- Handles caching and local storage of remote data

### RemoteClient
- Connects to remote THORChain gRPC
- Fetches data via gRPC query methods
- Implements retry logic and error handling

### Cache
- LRU cache for frequently accessed keys
- Thread-safe implementation
- Configurable size and eviction policies

## Configuration

The forking module is configured via CLI flags:

- `--fork.grpc`: Remote gRPC endpoint (e.g., "thornode.ninerealms.com:9090")
- `--fork.chain-id`: Remote chain ID
- `--fork.trust-height`: Initial trusted height for light client
- `--fork.trust-hash`: Initial trusted block hash
- `--fork.cache-size`: Cache size (default: 10000)
- `--fork.gas-cost`: Gas cost per remote fetch (default: 1000)

## Security

- Uses Tendermint light client for header verification
- Verifies Merkle proofs using ICS-23 specification
- Pins queries to specific block height for determinism
- Configurable trusting period and clock drift tolerance

## Performance

- LRU cache reduces redundant remote fetches
- Local storage eliminates repeated remote queries
- Gas accounting prevents abuse
- Configurable timeouts and retry logic

## Usage

The forking module is automatically enabled when forking configuration is provided. It's completely transparent to existing THORChain modules and keepers.

Example usage in development:
```bash
thornode start --fork.grpc=thornode.ninerealms.com:9090 --fork.chain-id=thorchain-mainnet-v1
```

## Testing

The module can be tested using the THORChain package with Kurtosis:

1. Build custom THORNode image with forking capabilities
2. Update network configuration to use the custom image
3. Deploy with Kurtosis and test account/contract interactions
4. Verify state consistency with mainnet
