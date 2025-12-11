package erc20

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/erc20/contract"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type ERC20CMD interface {
	Mint(to common.Address, amount *big.Int) (*types.Transaction, error)
	Approve(spender common.Address, amount *big.Int) (*types.Transaction, error)
	Transfer(to common.Address, amount *big.Int) (*types.Transaction, error)
	Allowance(owner common.Address, spender common.Address) (*big.Int, error)
	BalanceOf(account common.Address) (*big.Int, error)
}

type ERC20CMDImpl struct {
	conn    *ethclient.Client
	chainID int64
	pvtKey  *ecdsa.PrivateKey
	token   *contract.Myerc20
}

func NewERC20CMDImpl(conn *ethclient.Client, chainID int64, pvtKey *ecdsa.PrivateKey, token *contract.Myerc20) *ERC20CMDImpl {
	return &ERC20CMDImpl{
		conn,
		chainID,
		pvtKey,
		token,
	}
}

func (e *ERC20CMDImpl) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	// CreateTransactionSignerは元のコードにあるユーティリティ関数と仮定しています
	signer, err := CreateTransactionSigner(e.conn, e.chainID, e.pvtKey)
	if err != nil {
		return nil, err
	}

	tx, err := e.token.Mint(signer, to, amount)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (e *ERC20CMDImpl) Approve(spender common.Address, amount *big.Int) (*types.Transaction, error) {
	signer, err := CreateTransactionSigner(e.conn, e.chainID, e.pvtKey)
	if err != nil {
		return nil, err
	}

	tx, err := e.token.Approve(signer, spender, amount)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (e *ERC20CMDImpl) Transfer(to common.Address, amount *big.Int) (*types.Transaction, error) {
	signer, err := CreateTransactionSigner(e.conn, e.chainID, e.pvtKey)
	if err != nil {
		return nil, err
	}

	tx, err := e.token.Transfer(signer, to, amount)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (e *ERC20CMDImpl) Allowance(owner common.Address, spender common.Address) (*big.Int, error) {
	// 読み取り専用メソッドなのでsigner(opts)はnilで呼び出します
	amount, err := e.token.Allowance(nil, owner, spender)
	if err != nil {
		return nil, err
	}

	return amount, nil
}

func (e *ERC20CMDImpl) BalanceOf(account common.Address) (*big.Int, error) {
	balance, err := e.token.BalanceOf(nil, account)
	if err != nil {
		return nil, err
	}

	return balance, nil
}
