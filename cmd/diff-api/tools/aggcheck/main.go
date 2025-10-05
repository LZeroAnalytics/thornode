package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	agg "gitlab.com/thorchain/thornode/v3/cmd/diff-api/aggregator"
)

type storeWriteA struct {
	Store string `json:"store"`
	Op    string `json:"op"`
	Key   string `json:"key_hex"`
	Value string `json:"value_b64"`
}
type sincePayloadA struct {
	BaseHeight   int64         `json:"base_height"`
	TargetHeight int64         `json:"target_height"`
	StoreWrites  []storeWriteA `json:"store_writes"`
}

type storeWriteB struct {
	Store string `json:"store"`
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}
func asSlice(v any) ([]any, bool) {
	a, ok := v.([]any)
	return a, ok
}
func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func main() {
	in := flag.String("in", "", "path to /diffs/since JSON")
	storeFilter := flag.String("store", "wasm", "store to extract (default wasm)")
	debugKeys := flag.Int("debug-keys", 0, "print first N wasm keys with prefix and length, then exit")
	enrich := flag.Bool("enrich", false, "fetch wasm code_bytes from REST and attach")
	codeBase := flag.String("code-base", "https://thorchain.bloctopus.io/api", "REST base for code download")
	height := flag.Int64("height", 0, "target height for x-cosmos-block-height")
	flag.Parse()
	var inPath string
	if *in != "" {
		inPath = *in
	} else if flag.NArg() > 0 {
		inPath = flag.Arg(0)
	}
	if inPath == "" {
		fmt.Println("usage: aggcheck -in dump_state.json [-store wasm] [-debug-keys N]")
		os.Exit(1)
	}

	bz, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	var spa sincePayloadA
	if json.Unmarshal(bz, &spa) == nil && len(spa.StoreWrites) > 0 {
		if *debugKeys > 0 {
			n := *debugKeys
			fmt.Println("DEBUG first keys (sincePayloadA):")
			cnt := 0
			for _, w := range spa.StoreWrites {
				if w.Store != *storeFilter {
					continue
				}
				kb, err := hex.DecodeString(w.Key)
				if err != nil || len(kb) == 0 {
					continue
				}
				fmt.Printf("%02x len=%d key=%s\n", kb[0], len(kb), w.Key)
				cnt++
				if cnt >= n {
					return
				}
			}
		}
		var ws []agg.KVWrite
		for _, w := range spa.StoreWrites {
			if w.Store == "" {
				continue
			}
			if *storeFilter != "" && w.Store != *storeFilter {
				continue
			}
			ws = append(ws, agg.KVWrite{
				Store: w.Store,
				Op:    w.Op,
				Key:   w.Key,
				Value: w.Value,
			})
		}
		out, err := agg.AggregateAppState(ws)
		if err != nil {
			fmt.Fprintln(os.Stderr, "aggregate:", err)
			os.Exit(1)
		}
		if *enrich && *storeFilter == "wasm" && *height > 0 {
			if wasmAny, ok := out["wasm"]; ok {
				if wasm, ok := wasmAny.(map[string]any); ok {
					if codesAny, ok := wasm["codes"]; ok {
						var idx int
						var next func() (map[string]any, bool)
						if slice, ok := codesAny.([]map[string]any); ok && len(slice) > 0 {
							next = func() (map[string]any, bool) {
								if idx >= len(slice) {
									return nil, false
								}
								m := slice[idx]
								idx++
								return m, true
							}
						} else if sliceAny, ok := codesAny.([]any); ok && len(sliceAny) > 0 {
							next = func() (map[string]any, bool) {
								if idx >= len(sliceAny) {
									return nil, false
								}
								m, ok := sliceAny[idx].(map[string]any)
								idx++
								if !ok {
									return nil, false
								}
								return m, true
							}
						} else {
							next = nil
						}
						if next != nil {
							client := &http.Client{Timeout: 3 * time.Second}
							base := strings.TrimRight(*codeBase, "/")
							hh := strconv.FormatInt(*height, 10)
							log := os.Getenv("AGGCHECK_LOG_ENRICH") != ""
							for {
								rec, ok := next()
								if !ok {
									break
								}
								if _, exists := rec["code_bytes"]; exists {
									continue
								}
								var idStr string
								switch v := rec["code_id"].(type) {
								case string:
									idStr = v
								case float64:
									idStr = strconv.FormatInt(int64(v), 10)
								default:
									if n, ok := rec["code_id"].(json.Number); ok {
										idStr = string(n)
									}
								}
								if idStr == "" {
									continue
								}
								url := fmt.Sprintf("%s/cosmwasm/wasm/v1/code/%s", base, idStr)
								req, err := http.NewRequest("GET", url, nil)
								if err != nil {
									if log { fmt.Fprintf(os.Stderr, "enrich: newreq id=%s err=%v\n", idStr, err) }
									continue
								}
								req.Header.Set("x-cosmos-block-height", hh)
								resp, err := client.Do(req)
								if err != nil {
									if log { fmt.Fprintf(os.Stderr, "enrich: http id=%s err=%v\n", idStr, err) }
									continue
								}
								func() {
									defer resp.Body.Close()
									if resp.StatusCode != 200 {
										io.Copy(io.Discard, resp.Body)
										if log { fmt.Fprintf(os.Stderr, "enrich: non200 id=%s status=%d\n", idStr, resp.StatusCode) }
										return
									}
									var cbr struct{ Data string `json:"data"` }
									if err := json.NewDecoder(resp.Body).Decode(&cbr); err != nil || cbr.Data == "" {
										if log { fmt.Fprintf(os.Stderr, "enrich: decode/empty id=%s err=%v\n", idStr, err) }
										return
									}
									rec["code_bytes"] = cbr.Data
									if log { fmt.Fprintf(os.Stderr, "enrich: ok id=%s bytes=%d\n", idStr, len(cbr.Data)) }
								}()
							}
						}
					}
				}
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if *storeFilter == "wasm" {
			if m, ok := out["wasm"]; ok {
				_ = enc.Encode(m)
				return
			}
		}
		_ = enc.Encode(out)
		return
	}

	var root any
	if err := json.Unmarshal(bz, &root); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	var kvs []storeWriteB
	if m, ok := asMap(root); ok {
		if arr, ok := asSlice(m["writes"]); ok {
			for _, it := range arr {
				if mm, ok := asMap(it); ok {
					kvs = append(kvs, storeWriteB{
						Store: str(mm["store"]),
						Op:    str(mm["op"]),
						Key:   str(mm["key"]),
						Value: str(mm["value"]),
					})
				}
			}
		}
		if mods, ok := asMap(m["modules"]); ok {
			if sm, ok := asMap(mods[*storeFilter]); ok {
				if arr, ok := asSlice(sm["writes"]); ok {
					for _, it := range arr {
						if mm, ok := asMap(it); ok {
							s := str(mm["store"])
							if s == "" {
								s = *storeFilter
							}
							kvs = append(kvs, storeWriteB{
								Store: s,
								Op:    str(mm["op"]),
								Key:   str(mm["key"]),
								Value: str(mm["value"]),
							})
						}
					}
				}
			}
		}
		if arr, ok := asSlice(m["patches"]); ok {
			for _, it := range arr {
				if mm, ok := asMap(it); ok {
					kvs = append(kvs, storeWriteB{
						Store: str(mm["store"]),
						Op:    str(mm["op"]),
						Key:   str(mm["key"]),
						Value: str(mm["value"]),
					})
				}
			}
		}
	}

	if len(kvs) == 0 {
		fmt.Fprintln(os.Stderr, "no kv writes found")
		os.Exit(1)
	}
	if *debugKeys > 0 {
		fmt.Println("DEBUG first keys (generic root):")
		cnt := 0
		for _, k := range kvs {
			if k.Store != *storeFilter {
				continue
			}
			kb, err := hex.DecodeString(k.Key)
			if err != nil || len(kb) == 0 {
				continue
			}
			fmt.Printf("%02x len=%d key=%s\n", kb[0], len(kb), k.Key)
			cnt++
			if cnt >= *debugKeys {
				return
			}
		}
	}
	var ws []agg.KVWrite
	for _, k := range kvs {
		if k.Store == "" {
			continue
		}
		if *storeFilter != "" && k.Store != *storeFilter {
			continue
		}
		ws = append(ws, agg.KVWrite{
			Store: k.Store,
			Op:    k.Op,
			Key:   k.Key,
			Value: k.Value,
		})
	}
	out, err := agg.AggregateAppState(ws)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aggregate:", err)
		os.Exit(1)
	}
	if *enrich && *storeFilter == "wasm" {
		if wasmAny, ok := out["wasm"]; ok {
			if wasm, ok := wasmAny.(map[string]any); ok {
				if codesAny, ok := wasm["codes"]; ok {
					var idx int
					var next func() (map[string]any, bool)
					if slice, ok := codesAny.([]map[string]any); ok && len(slice) > 0 {
						next = func() (map[string]any, bool) {
							if idx >= len(slice) {
								return nil, false
							}
							m := slice[idx]
							idx++
							return m, true
						}
					} else if sliceAny, ok := codesAny.([]any); ok && len(sliceAny) > 0 {
						next = func() (map[string]any, bool) {
							if idx >= len(sliceAny) {
								return nil, false
							}
							m, ok := sliceAny[idx].(map[string]any)
							idx++
							if !ok {
								return nil, false
							}
							return m, true
						}
					} else {
						next = nil
					}
					if next != nil {
						client := &http.Client{Timeout: 3 * time.Second}
						base := strings.TrimRight(*codeBase, "/")
						hh := strconv.FormatInt(*height, 10)
						for {
							rec, ok := next()
							if !ok {
								break
							}
							if _, exists := rec["code_bytes"]; exists {
								continue
							}
							var idStr string
							switch v := rec["code_id"].(type) {
							case string:
								idStr = v
							case float64:
								idStr = strconv.FormatInt(int64(v), 10)
							default:
								if n, ok := rec["code_id"].(json.Number); ok {
									idStr = string(n)
								}
							}
							if idStr == "" {
								continue
							}
								url := fmt.Sprintf("%s/cosmwasm/wasm/v1/code/%s", base, idStr)
								req, err := http.NewRequest("GET", url, nil)
							if err != nil {
								continue
							}
							if *height > 0 {
								req.Header.Set("x-cosmos-block-height", hh)
							}
							resp, err := client.Do(req)
							if err != nil {
								continue
							}
							func() {
								defer resp.Body.Close()
								if resp.StatusCode != 200 {
									io.Copy(io.Discard, resp.Body)
									return
								}
								var cbr struct{ Data string `json:"data"` }
								if err := json.NewDecoder(resp.Body).Decode(&cbr); err != nil || cbr.Data == "" {
									return
								}
								rec["code_bytes"] = cbr.Data
							}()
						}
					}
				}
			}
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if *storeFilter == "wasm" {
		if m, ok := out["wasm"]; ok {
			_ = enc.Encode(m)
			return
		}
	}
	_ = enc.Encode(out)
}
