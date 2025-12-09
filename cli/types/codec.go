package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	authtypes "github.com/datachainlab/cross/x/core/auth/types"
)

// RegisterInterfaces register the ibc transfer module interfaces to protobuf
// Any.
func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*authtypes.AuthExtensionVerifier)(nil),
		&BesuAuthExtension{},
	)
}
