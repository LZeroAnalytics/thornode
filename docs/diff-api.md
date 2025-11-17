diff-api server

Build
- go build ./cmd/diff-api

Runtime env
- THOR_DIFF_OUT: directory containing per-block diff JSONs (from diff-writer), default /root/.thornode/diffs
- THOR_DIFF_BASE_HEIGHT: base height (optional; inferred from files if unset)
- DIFF_API_ADDR: listen address (default :8080)
- CHECKPOINT_INTERVAL: build cumulative checkpoints every N blocks (default 1000; set 0 to disable)
- CHECKPOINT_DIR: directory for checkpoints (default $THOR_DIFF_OUT/checkpoints)
- SHARD_WIDTH: read sharded per-block diffs at $THOR_DIFF_OUT/{prefix}/{height}.json, where prefix=first SHARD_WIDTH digits (default 0=disabled)
- ENABLE_GZIP: if 1 (default), compress JSON responses when client sends Accept-Encoding: gzip

Endpoints
- GET /diffs/meta → { base_height, min_height, max_height, count, sample }
- GET /diffs/height/{H} and GET /diffs/block/{H} → return the per-block diff file for height H (supports .json and .json.gz; sharded or flat)
- GET /diffs/since/{H} → returns cumulative KV patch from (base_height, H], using last-write-wins per (store,key). If checkpoints exist, serves “nearest checkpoint + tail” for near O(1).
- GET /bloctopus/diffs/patch/since/{H} → returns high-level app_state module patches: { base_height, target_height, app_state: {<module>: {...}} }, gzip-enabled.
- POST /diffs/validate → validate a list of kvWrite items (decode checks)

Behavior
- Deterministic output: cumulative patches are stably sorted by store then key.
- Checkpoints: on startup, the server scans available heights and writes checkpoints every CHECKPOINT_INTERVAL blocks to $CHECKPOINT_DIR. Existing checkpoints are reused automatically by /diffs/since/{H}.
- Sharding: server transparently reads flat and sharded layouts; you can enable sharding when generating diffs in the future without breaking compatibility.
- Gzip: responses are compressed when the client requests it (and ENABLE_GZIP=1).

Checkpoint tuning
- Smaller intervals (e.g., 100–1,000): fastest /diffs/since responses; higher CPU/disk/inodes to build/keep checkpoints.
- Larger intervals (e.g., 5,000–10,000): lower overhead; slower /diffs/since for large height jumps.
- Choose based on latency vs. resource trade-off. Typical defaults: 1,000 for low latency; 5,000 if conserving resources.

Applying patches to base genesis efficiently
- The KV patch represents final values per (store,key) after folding from (base, H]. For O(1)-ish apply over a large base file:
  - Recommended: build a KV index representation of the base state once (keyed by (store,key)), then stream-apply the patch to that index and re-materialize JSON if required.
  - Directly rewriting a 700MB monolithic JSON in place is slow and fragile. A KV index approach is faster and single-pass.
- The patch entries:
  - {store, op: "set"|"delete", key_hex, value_b64?}. For "delete", value_b64 is empty.

Docker
- The Dockerfile builds and includes /usr/local/bin/diff-api in the thornode image.
- Example run:
  docker run --rm -it -p 8080:8080 \
    -v /thornode/diffs:/root/.thornode/diffs \
    -e THOR_DIFF_OUT=/root/.thornode/diffs \
    -e THOR_DIFF_BASE_HEIGHT=23010004 \
    -e CHECKPOINT_INTERVAL=1000 \
    -e ENABLE_GZIP=1 \
    tiljordan/thornode-forking:local /usr/local/bin/diff-api

Verification
- curl localhost:8080/diffs/meta
- curl localhost:8080/diffs/height/23011828
- curl localhost:8080/diffs/since/23011828
- ls /thornode/diffs/checkpoints | head  # after some time if checkpoints enabled
