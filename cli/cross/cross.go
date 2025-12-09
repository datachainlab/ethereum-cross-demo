package cross

import (
	"crypto/ecdsa"

	"github.com/datachainlab/fabric-besu-cross-demo/cmds/erc20/cross/contract"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CrossCMD interface {
	InitiateTx(msg contract.MsgInitiateTxData) (*types.Transaction, error)
	ExtSignTx(msg contract.MsgExtSignTxData) (*types.Transaction, error)
	CoordinatorState(req contract.QueryCoordinatorStateRequestData) (contract.QueryCoordinatorStateResponseData, error)
	SelfXCC() (contract.QuerySelfXCCResponseData, error)
	// TxAuthState is generated as a transaction in the binding, though it might be a view in Solidity
	TxAuthState(req contract.QueryTxAuthStateRequestData) (*types.Transaction, error)
}

type CrossCMDImpl struct {
	conn    *ethclient.Client
	chainID int64
	pvtKey  *ecdsa.PrivateKey
	cross   *contract.Crosssimplemodule
}

func NewCrossCMDImpl(conn *ethclient.Client, chainID int64, pvtKey *ecdsa.PrivateKey, cross *contract.Crosssimplemodule) *CrossCMDImpl {
	return &CrossCMDImpl{
		conn,
		chainID,
		pvtKey,
		cross,
	}
}

func (c *CrossCMDImpl) InitiateTx(msg contract.MsgInitiateTxData) (*types.Transaction, error) {
	signer, err := CreateTransactionSigner(c.conn, c.chainID, c.pvtKey)
	if err != nil {
		return nil, err
	}

	tx, err := c.cross.InitiateTx(signer, msg)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (c *CrossCMDImpl) ExtSignTx(msg contract.MsgExtSignTxData) (*types.Transaction, error) {
	signer, err := CreateTransactionSigner(c.conn, c.chainID, c.pvtKey)
	if err != nil {
		return nil, err
	}

	tx, err := c.cross.ExtSignTx(signer, msg)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (c *CrossCMDImpl) CoordinatorState(req contract.QueryCoordinatorStateRequestData) (contract.QueryCoordinatorStateResponseData, error) {
	// Read-only call
	return c.cross.CoordinatorState(nil, req)
}

func (c *CrossCMDImpl) SelfXCC() (contract.QuerySelfXCCResponseData, error) {
	// Read-only call
	return c.cross.SelfXCC(nil)
}

func (c *CrossCMDImpl) TxAuthState(req contract.QueryTxAuthStateRequestData) (*types.Transaction, error) {
	signer, err := CreateTransactionSigner(c.conn, c.chainID, c.pvtKey)
	if err != nil {
		return nil, err
	}

	// Generated binding defines this as a transaction
	tx, err := c.cross.TxAuthState(signer, req)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
