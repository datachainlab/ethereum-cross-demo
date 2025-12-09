package cross

import (
	"context"
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
)

func Connect(url string) (*ethclient.Client, error) {
	conn, err := ethclient.Dial(url)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to connect to the Ethereum client")
	}
	return conn, nil
}

func PvtKeyToPubAddress(pvtKey *ecdsa.PrivateKey) (common.Address, error) {
	publicKey := pvtKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}, errors.New("Failed to publicKeyECDSA")
	}

	return crypto.PubkeyToAddress(*publicKeyECDSA), nil
}

func CreateTransactionSigner(client *ethclient.Client, chainID int64, pvtKey *ecdsa.PrivateKey) (*bind.TransactOpts, error) {
	fromAddress, err := PvtKeyToPubAddress(pvtKey)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to calculate")
	}
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to nonce")
	}
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to gasPrice")
	}

	signer, err := bind.NewKeyedTransactorWithChainID(pvtKey, big.NewInt(chainID))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create authorized transactor")
	}
	signer.Nonce = big.NewInt(int64(nonce))
	signer.Value = big.NewInt(0)
	signer.GasLimit = 5000000
	signer.GasPrice = gasPrice
	signer.Context = context.Background()

	return signer, nil
}
