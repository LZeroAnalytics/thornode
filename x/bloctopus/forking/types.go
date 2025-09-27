package forking

type RemoteConfig struct {
	GRPC string
}

type Options struct {
	Enabled bool
	Config  RemoteConfig
}
