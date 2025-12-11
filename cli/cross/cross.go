package cross

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"strings"

	contract "github.com/datachainlab/anvil-cross-demo/cmds/erc20/contract/crosssimplemodule"
	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/eth"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

type CrossCMD interface {
	InitiateTx(msg contract.MsgInitiateTxData) (*types.Transaction, error)
	ExtSignTx(msg contract.MsgExtSignTxData) (*types.Transaction, error)
	GetTxInitiatedEvent(ctx context.Context, tx *types.Transaction) (*contract.CrosssimplemoduleTxInitiated, error)
	QueryTxAuthState(ctx context.Context, req contract.QueryTxAuthStateRequestData) (contract.QueryTxAuthStateResponseData, error)
	QueryCoordinatorState(ctx context.Context, req contract.QueryCoordinatorStateRequestData) (contract.QueryCoordinatorStateResponseData, error)
}

type CrossCMDImpl struct {
	conn    *ethclient.Client
	chainID int64
	pvtKey  *ecdsa.PrivateKey
	cross   *contract.Crosssimplemodule
	address common.Address
}

func NewCrossCMDImpl(conn *ethclient.Client, chainID int64, pvtKey *ecdsa.PrivateKey, cross *contract.Crosssimplemodule, address common.Address) *CrossCMDImpl {
	return &CrossCMDImpl{
		conn,
		chainID,
		pvtKey,
		cross,
		address,
	}
}

func (c *CrossCMDImpl) InitiateTx(msg contract.MsgInitiateTxData) (*types.Transaction, error) {
	signer, err := eth.CreateTransactionSigner(c.conn, c.chainID, c.pvtKey)
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
	signer, err := eth.CreateTransactionSigner(c.conn, c.chainID, c.pvtKey)
	if err != nil {
		return nil, err
	}

	tx, err := c.cross.ExtSignTx(signer, msg)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (c *CrossCMDImpl) GetTxInitiatedEvent(ctx context.Context, tx *types.Transaction) (*contract.CrosssimplemoduleTxInitiated, error) {
	receipt, err := bind.WaitMined(ctx, c.conn, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for tx receipt: %w", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		reason, err := getRevertReason(ctx, c.conn, tx, receipt)
		if err != nil {
			return nil, fmt.Errorf("transaction failed (status: 0) and failed to retrieve reason: %v", err)
		}
		return nil, fmt.Errorf("transaction failed (status: 0): %s", reason)
	}

	for _, l := range receipt.Logs {
		event, err := c.cross.ParseTxInitiated(*l)
		if err == nil {
			return event, nil
		}
	}

	return nil, fmt.Errorf("transaction succeeded but TxInitiated event not found")
}

func (c *CrossCMDImpl) QueryTxAuthState(ctx context.Context, req contract.QueryTxAuthStateRequestData) (contract.QueryTxAuthStateResponseData, error) {
	var emptyResp contract.QueryTxAuthStateResponseData

	contractABI, err := abi.JSON(strings.NewReader(contract.CrosssimplemoduleMetaData.ABI))
	if err != nil {
		return emptyResp, fmt.Errorf("failed to parse ABI: %w", err)
	}

	data, err := contractABI.Pack("txAuthState", req)
	if err != nil {
		return emptyResp, fmt.Errorf("failed to pack arguments: %w", err)
	}

	from := crypto.PubkeyToAddress(c.pvtKey.PublicKey)
	msg := ethereum.CallMsg{
		From: from,
		To:   &c.address,
		Data: data,
	}
	result, err := c.conn.CallContract(ctx, msg, nil)
	if err != nil {
		return emptyResp, fmt.Errorf("failed to call contract: %w", err)
	}

	var outputWrapper struct {
		Ret0 contract.QueryTxAuthStateResponseData
	}

	err = contractABI.UnpackIntoInterface(&outputWrapper, "txAuthState", result)
	if err != nil {
		return emptyResp, fmt.Errorf("failed to unpack response: %w", err)
	}

	return outputWrapper.Ret0, nil
}

func (c *CrossCMDImpl) QueryCoordinatorState(ctx context.Context, req contract.QueryCoordinatorStateRequestData) (contract.QueryCoordinatorStateResponseData, error) {
	var emptyResp contract.QueryCoordinatorStateResponseData

	contractABI, err := abi.JSON(strings.NewReader(contract.CrosssimplemoduleMetaData.ABI))
	if err != nil {
		return emptyResp, fmt.Errorf("failed to parse ABI: %w", err)
	}

	data, err := contractABI.Pack("coordinatorState", req)
	if err != nil {
		return emptyResp, fmt.Errorf("failed to pack arguments: %w", err)
	}

	from := crypto.PubkeyToAddress(c.pvtKey.PublicKey)
	msg := ethereum.CallMsg{
		From: from,
		To:   &c.address,
		Data: data,
	}
	result, err := c.conn.CallContract(ctx, msg, nil)
	if err != nil {
		return emptyResp, fmt.Errorf("failed to call contract: %w", err)
	}

	var outputWrapper struct {
		Ret0 contract.QueryCoordinatorStateResponseData
	}

	err = contractABI.UnpackIntoInterface(&outputWrapper, "coordinatorState", result)
	if err != nil {
		return emptyResp, fmt.Errorf("failed to unpack response: %w", err)
	}

	return outputWrapper.Ret0, nil
}

func getRevertReason(ctx context.Context, conn *ethclient.Client, tx *types.Transaction, receipt *types.Receipt) (string, error) {
	from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return "", fmt.Errorf("failed to recover sender: %w", err)
	}

	callMsg := ethereum.CallMsg{
		From:     from,
		To:       tx.To(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasPrice(),
		Value:    tx.Value(),
		Data:     tx.Data(),
	}

	_, err = conn.CallContract(ctx, callMsg, receipt.BlockNumber)
	if err != nil {
		var dataErr rpc.DataError
		if errors.As(err, &dataErr) {
			data := dataErr.ErrorData()
			if dataStr, ok := data.(string); ok {
				return parseRevertData(dataStr), nil
			}
		}

		errMsg := err.Error()
		if strings.Contains(errMsg, "0x") {
			parts := strings.Split(errMsg, "0x")
			if len(parts) > 1 {
				hexData := "0x" + parts[len(parts)-1]
				hexData = strings.Fields(hexData)[0]
				return parseRevertData(hexData), nil
			}
		}
		return fmt.Sprintf("Error from CallContract: %s", errMsg), nil
	}
	return "No error returned from CallContract (unexpected for failed tx)", nil
}

func parseRevertData(dataStr string) string {
	if !strings.HasPrefix(dataStr, "0x") {
		return fmt.Sprintf("Raw Data: %s", dataStr)
	}
	dataBytes, err := hexutil.Decode(dataStr)
	if err != nil {
		return fmt.Sprintf("Raw Data (Hex): %s", dataStr)
	}
	if len(dataBytes) < 4 {
		return fmt.Sprintf("Raw Data (Too short): %s", dataStr)
	}

	selector := hex.EncodeToString(dataBytes[:4])

	return fmt.Sprintf("Custom Error (Selector: 0x%s)", selector)
}
