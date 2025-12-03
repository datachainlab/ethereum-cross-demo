package config

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
)

type Config struct {
	BlockchainHost     string `mapstructure:"blockchain-host"`
	ChainID            int64  `mapstructure:"chain-id"`
	CrossModuleAddress string `mapstructure:"cross-module-address"`
	ERC20TokenAddress  string `mapstructure:"erc20-token-address"`
	WatchTimeoutSec    uint   `mapstructure:"watch-timeout-sec"`
	WalletFile         string `mapstructure:"wallet-file"`
}

func (c *Config) PrivateKey() (*ecdsa.PrivateKey, error) {
	pvt, err := crypto.LoadECDSA(c.WalletFile)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to read")
	}
	return pvt, nil
}
