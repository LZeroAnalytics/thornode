package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	gogoproto "github.com/cosmos/gogoproto/proto"
)

type storeWrite struct {
	Store string `json:"store"`
	Op    string `json:"op"`
	Key   string `json:"key_hex"`
	Value string `json:"value_b64"`
}
type sincePayload struct {
	StoreWrites []storeWrite `json:"store_writes"`
}

func makeLocalCodec() codec.Codec {
	ir := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(ir)
	authtypes.RegisterInterfaces(ir)
	vestingtypes.RegisterInterfaces(ir)
	return codec.NewProtoCodec(ir)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: accprobe <since.json>")
		os.Exit(1)
	}
	bz, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var sp sincePayload
	if err := json.Unmarshal(bz, &sp); err != nil {
		panic(err)
	}
	cdc := makeLocalCodec()
	okCount := 0
	failCount := 0
	printed := 0
	for _, w := range sp.StoreWrites {
		if w.Store != authtypes.StoreKey || w.Op != "set" || w.Value == "" {
			continue
		}
		vb, err := base64.StdEncoding.DecodeString(w.Value)
		if err != nil || len(vb) == 0 {
			failCount++
			continue
		}
		var anyMsg codectypes.Any
		errAny := cdc.Unmarshal(vb, &anyMsg)
		if errAny != nil && printed < 3 {
			fmt.Printf("unmarshal->Any failed for key=%s: %v (len=%d)\n", w.Key, errAny, len(vb))
			printed++
		}
		if errAny == nil && printed < 5 {
			fmt.Printf("Any type_url=%q, value_len=%d\n", anyMsg.TypeUrl, len(anyMsg.Value))
			printed++
		}
		var ai authtypes.AccountI
		if errAny == nil {
			if err := cdc.UnpackAny(&anyMsg, &ai); err == nil && ai != nil {
				okCount++
				if printed < 1 {
					jb, _ := cdc.MarshalJSON(ai)
					fmt.Printf("sample account: %s\n", string(jb))
					printed++
				}
				continue
			}
		}
		if errAny == nil && len(anyMsg.Value) > 0 {
			var ba authtypes.BaseAccount
			if err := cdc.Unmarshal(anyMsg.Value, &ba); err == nil && ba.Address != "" {
				okCount++
				if printed < 1 {
					jb, _ := cdc.MarshalJSON(&ba)
					fmt.Printf("sample account: %s\n", string(jb))
					printed++
				}
				continue
			}
			if printed < 3 {
				fmt.Printf("direct BaseAccount unmarshal from Any.Value failed for key=%s\n", w.Key)
				printed++
			}
		}
		{
			var gAny codectypes.Any
			if err := gogoproto.Unmarshal(vb, &gAny); err == nil && gAny.TypeUrl != "" && len(gAny.Value) > 0 {
				if printed < 5 {
					fmt.Printf("gogo Any type_url=%q, value_len=%d\n", gAny.TypeUrl, len(gAny.Value))
					printed++
				}
				var ba authtypes.BaseAccount
				if err := gogoproto.Unmarshal(gAny.Value, &ba); err == nil && ba.Address != "" {
					okCount++
					if printed < 1 {
						jb, _ := cdc.MarshalJSON(&ba)
						fmt.Printf("sample account: %s\n", string(jb))
						printed++
					}
					continue
				}
			}
		}
		failCount++
	}
	fmt.Printf("acc decode ok=%d fail=%d\n", okCount, failCount)
}
