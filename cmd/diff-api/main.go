package main

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gitlab.com/thorchain/thornode/v3/cmd/diff-api/aggregator"
)

type kvWrite struct {
	Store string `json:"store"`
	Op    string `json:"op"`
	Key   string `json:"key_hex"`
	Value string `json:"value_b64,omitempty"`
}

type blockDiff struct {
	BaseHeight  int64     `json:"base_height"`
	Height      int64     `json:"height"`
	Time        string    `json:"time"`
	StoreWrites []kvWrite `json:"store_writes"`
}

type cumulativeResponse struct {
	BaseHeight   int64     `json:"base_height"`
	TargetHeight int64     `json:"target_height"`
	StoreWrites  []kvWrite `json:"store_writes"`
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if x, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return x
		}
	}
	return def
}

func parseHeightParam(s string) (int64, error) {
	h, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || h <= 0 {
		return 0, fmt.Errorf("invalid height: %q", s)
	}
	return h, nil
}

func shardPrefix(height int64, width int) string {
	if width <= 0 {
		return ""
	}
	s := strconv.FormatInt(height, 10)
	if len(s) <= width {
		return s
	}
	return s[:width]
}

func openMaybeGzip(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".gz") {
		gz, gzErr := gzip.NewReader(f)
		if gzErr != nil {
			f.Close()
			return nil, gzErr
		}
		return struct {
			io.Reader
			io.Closer
		}{Reader: gz, Closer: f}, nil
	}
	return f, nil
}

func heightPaths(dir string, height int64, shardWidth int) []string {
	base := filepath.Join(dir, fmt.Sprintf("%d.json", height))
	gz := base + ".gz"
	if shardWidth > 0 {
		prefix := shardPrefix(height, shardWidth)
		baseSh := filepath.Join(dir, prefix, fmt.Sprintf("%d.json", height))
		return []string{baseSh, baseSh + ".gz", base, gz}
	}
	return []string{base, gz}
}

func readBlockDiff(dir string, height int64) (*blockDiff, error) {
	for _, p := range heightPaths(dir, height, envInt("SHARD_WIDTH", 0)) {
		rc, err := openMaybeGzip(p)
		if err != nil {
			continue
		}
		defer rc.Close()
		dec := json.NewDecoder(rc)
		var bd blockDiff
		if err := dec.Decode(&bd); err != nil {
			return nil, err
		}
		return &bd, nil
	}
	return nil, os.ErrNotExist
}

func listAvailableHeights(dir string) ([]int64, error) {
	shardW := envInt("SHARD_WIDTH", 0)
	var ents []os.DirEntry
	var err error
	if shardW <= 0 {
		ents, err = os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
	} else {
		top, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range top {
			if !e.IsDir() {
				continue
			}
			sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			ents = append(ents, sub...)
		}
	}
	out := make([]int64, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		n := strings.TrimSuffix(name, ".json")
		if strings.HasSuffix(n, ".gz") {
			continue
		}
		if h, err := strconv.ParseInt(n, 10, 64); err == nil && h > 0 {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func writeJSON(w http.ResponseWriter, r *http.Request, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enableGzip := envInt("ENABLE_GZIP", 1) == 1
	if enableGzip && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		enc := json.NewEncoder(gz)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func foldCumulativeFromTo(dir string, start, target int64, last map[string]kvWrite) error {
	for h := start; h <= target; h++ {
		bd, err := readBlockDiff(dir, h)
		if err != nil {
			continue
		}
		for _, w := range bd.StoreWrites {
			k := w.Store + "\x00" + w.Key
			last[k] = w
		}
	}
	return nil
}

func sortPatch(m map[string]kvWrite) []kvWrite {
	ws := make([]kvWrite, 0, len(m))
	for _, w := range m {
		ws = append(ws, w)
	}
	sort.Slice(ws, func(i, j int) bool {
		if ws[i].Store != ws[j].Store {
			return ws[i].Store < ws[j].Store
		}
		return ws[i].Key < ws[j].Key
	})
	return ws
}

func loadCheckpoint(dir string, h int64) (map[string]kvWrite, bool) {
	path := filepath.Join(dir, fmt.Sprintf("%d.json", h))
	rc, err := openMaybeGzip(path)
	if err != nil {
		return nil, false
	}
	defer rc.Close()
	var resp cumulativeResponse
	if err := json.NewDecoder(rc).Decode(&resp); err != nil {
		return nil, false
	}
	m := make(map[string]kvWrite, len(resp.StoreWrites))
	for _, w := range resp.StoreWrites {
		m[w.Store+"\x00"+w.Key] = w
	}
	return m, true
}

func floorCheckpoint(checkDir string, base, target int64, interval int) int64 {
	if interval <= 0 {
		return 0
	}
	c := (target / int64(interval)) * int64(interval)
	if c <= base {
		return 0
	}
	if _, ok := loadCheckpoint(checkDir, c); ok {
		return c
	}
	return 0
}

func foldCumulative(dir string, base, target int64) (cumulativeResponse, error) {
	if target <= base {
		return cumulativeResponse{}, errors.New("target must be > base")
	}
	last := make(map[string]kvWrite)
	checkDir := envStr("CHECKPOINT_DIR", filepath.Join(dir, "checkpoints"))
	interval := envInt("CHECKPOINT_INTERVAL", 1000)

	if c := floorCheckpoint(checkDir, base, target, interval); c > 0 {
		if m, ok := loadCheckpoint(checkDir, c); ok {
			for k, v := range m {
				last[k] = v
			}
			_ = foldCumulativeFromTo(dir, c+1, target, last)
		} else {
			_ = foldCumulativeFromTo(dir, base+1, target, last)
		}
	} else {
		_ = foldCumulativeFromTo(dir, base+1, target, last)
	}

	return cumulativeResponse{
		BaseHeight:   base,
		TargetHeight: target,
		StoreWrites:  sortPatch(last),
	}, nil
}

func handleDiffSince(outDir string, base int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 {
			http.Error(w, "use /diffs/since/{height}", http.StatusBadRequest)
			return
		}
		h, err := parseHeightParam(parts[2])
			if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := foldCumulative(outDir, base, h)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, r, resp)
	}
}
func handlePatchSince(outDir string, base int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 5 {
			http.Error(w, "use /bloctopus/diffs/patch/since/{height}", http.StatusBadRequest)
			return
		}
		h, err := parseHeightParam(parts[4])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := foldCumulative(outDir, base, h)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		kvs := make([]aggregator.KVWrite, 0, len(resp.StoreWrites))
		for _, wv := range resp.StoreWrites {
			kvs = append(kvs, aggregator.KVWrite{
				Store: wv.Store,
				Op:    wv.Op,
				Key:   wv.Key,
				Value: wv.Value,
			})
		}
		appState, err := aggregator.AggregateAppState(kvs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type patchResp struct {
			BaseHeight   int64                `json:"base_height"`
			TargetHeight int64                `json:"target_height"`
			AppState     map[string]any       `json:"app_state"`
		}
		writeJSON(w, r, patchResp{
			BaseHeight:   resp.BaseHeight,
			TargetHeight: resp.TargetHeight,
			AppState:     appState,
		})
	}
}


func handleDiffHeight(outDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 {
			http.Error(w, "use /diffs/height/{height}", http.StatusBadRequest)
			return
		}
		h, err := parseHeightParam(parts[2])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var rc io.ReadCloser
		var opened bool
		for _, p := range heightPaths(outDir, h, envInt("SHARD_WIDTH", 0)) {
			rcc, err := openMaybeGzip(p)
			if err != nil {
				continue
			}
			rc = rcc
			opened = true
			break
		}
		if !opened {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer func() {
			if rc != nil {
				rc.Close()
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, rc)
	}
}

func handleMeta(outDir string, base int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hs, err := listAvailableHeights(outDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type meta struct {
			BaseHeight int64   `json:"base_height"`
			Min        int64   `json:"min_height"`
			Max        int64   `json:"max_height"`
			Count      int     `json:"count"`
			Sample     []int64 `json:"sample"`
		}
		m := meta{BaseHeight: base, Count: len(hs)}
		if len(hs) > 0 {
			m.Min = hs[0]
			m.Max = hs[len(hs)-1]
			if len(hs) > 10 {
				m.Sample = append(m.Sample, hs[:5]...)
				m.Sample = append(m.Sample, hs[len(hs)-5:]...)
			} else {
				m.Sample = hs
			}
		}
		writeJSON(w, r, m)
	}
}

func handleValidatePatch() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ws []kvWrite
		if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type res struct {
			Count int    `json:"count"`
			OK    bool   `json:"ok"`
			Err   string `json:"err,omitempty"`
		}
		for _, wv := range ws {
			if _, err := hex.DecodeString(wv.Key); err != nil {
				writeJSON(w, r, res{OK: false, Err: "bad key_hex"})
				return
			}
			if wv.Op != "set" && wv.Op != "delete" {
				writeJSON(w, r, res{OK: false, Err: "bad op"})
				return
			}
			if wv.Op == "set" && wv.Value != "" {
				if _, err := base64.StdEncoding.DecodeString(wv.Value); err != nil {
					writeJSON(w, r, res{OK: false, Err: "bad value_b64"})
					return
				}
			}
		}
		writeJSON(w, r, res{OK: true, Count: len(ws)})
	}
}

func buildCheckpoints(outDir string, base int64) {
	interval := envInt("CHECKPOINT_INTERVAL", 1000)
	if interval <= 0 {
		return
	}
	checkDir := envStr("CHECKPOINT_DIR", filepath.Join(outDir, "checkpoints"))
	_ = os.MkdirAll(checkDir, 0o755)
	hs, err := listAvailableHeights(outDir)
	if err != nil || len(hs) == 0 {
		return
	}
	last := make(map[string]kvWrite)
	start := base + 1
	for _, h := range hs {
		if h < start {
			continue
		}
		bd, err := readBlockDiff(outDir, h)
		if err != nil {
			continue
		}
		for _, w := range bd.StoreWrites {
			last[w.Store+"\x00"+w.Key] = w
		}
		if h%int64(interval) == 0 {
			cp := cumulativeResponse{
				BaseHeight:   base,
				TargetHeight: h,
				StoreWrites:  sortPatch(last),
			}
			tmp := filepath.Join(checkDir, fmt.Sprintf("%d.json.tmp", h))
			out := filepath.Join(checkDir, fmt.Sprintf("%d.json", h))
			f, err := os.Create(tmp)
			if err != nil {
				continue
			}
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			_ = enc.Encode(cp)
			_ = f.Close()
			_ = os.Rename(tmp, out)
		}
	}
}

func main() {
	outDir := envStr("THOR_DIFF_OUT", "/root/.thornode/diffs")
	baseStr := envStr("THOR_DIFF_BASE_HEIGHT", "")
	var base int64
	if baseStr != "" {
		if v, err := strconv.ParseInt(baseStr, 10, 64); err == nil {
			base = v
		}
	}
	if base == 0 {
		if hs, err := listAvailableHeights(outDir); err == nil && len(hs) > 0 {
			if bd, err := readBlockDiff(outDir, hs[0]); err == nil {
				base = bd.BaseHeight
			}
		}
	}

	go buildCheckpoints(outDir, base)

	mux := http.NewServeMux()
	mux.Handle("/diffs/since/", handleDiffSince(outDir, base))
	mux.Handle("/diffs/height/", handleDiffHeight(outDir))
	mux.Handle("/diffs/block/", handleDiffHeight(outDir))
	mux.Handle("/diffs/meta", handleMeta(outDir, base))
	mux.Handle("/diffs/validate", handleValidatePatch())
	mux.Handle("/bloctopus/diffs/patch/since/", handlePatchSince(outDir, base))

	addr := envStr("DIFF_API_ADDR", ":8080")
	fmt.Printf("diff-api listening on %s, out=%s, base=%d\n", addr, outDir, base)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}
