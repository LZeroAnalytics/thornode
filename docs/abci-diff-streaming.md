ABCI diff-writer plugin (per-block KV diffs)

How to enable
- In ~/.thornode/config/app.toml:
[streaming]
[streaming.abci]
keys = ["*"]
plugin = "abci_v1"
stop-node-on-err = false

Runtime env
- COSMOS_SDK_ABCI=/usr/local/bin/diff-writer
- Optional:
  - THOR_DIFF_OUT=/root/.thornode/diffs
  - THOR_DIFF_BASE_HEIGHT=23010393

Docker build (local)
- From repo root:
  docker build -t tiljordan/thornode-forking:local -f build/docker/Dockerfile .

Run example
docker run --rm -it --name thornode \
  -p 27147:27147 -p 27146:27146 -p 1317:1317 -p 9090:9090 \
  -v /thornode/config:/root/.thornode/config \
  -v /thornode/data:/root/.thornode/data \
  -v /thornode/diffs:/root/.thornode/diffs \
  -e COSMOS_SDK_ABCI=/usr/local/bin/diff-writer \
  -e THOR_DIFF_BASE_HEIGHT=23010393 \
  tiljordan/thornode-forking:local start

Output format
/root/.thornode/diffs/{height}.json
{
  "base_height": 23010393,
  "height": 23010394,
  "time": "2025-10-03T12:34:56.789Z",
  "app_hash_after": "ABCDEF...",
  "store_writes": [
    {"store":"bank","op":"set","key_hex":"...","value_b64":"..."},
    {"store":"thorchain","op":"delete","key_hex":"..."}
  ]
}

Verification
- curl http://localhost:27147/status
- curl http://localhost:1317/cosmos/bank/v1beta1/balances/{address}
- curl http://localhost:1317/thorchain/pools
- ls /thornode/diffs | tail
