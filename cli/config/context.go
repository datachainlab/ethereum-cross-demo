package config

import (
	"github.com/cosmos/cosmos-sdk/codec"
)

type Context struct {
	Config *Config
	Codec  codec.ProtoCodecMarshaler
}
