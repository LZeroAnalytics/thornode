package forking

import "github.com/spf13/pflag"

const (
	flagForkingEnabled = "forking.enabled"
	flagForkingGRPC    = "forking.grpc"
)

func AddFlags(fs *pflag.FlagSet) {
	fs.Bool(flagForkingEnabled, true, "enable forking")
	fs.String(flagForkingGRPC, "grpc.thor.pfc.zone:443", "forking grpc endpoint")
}
