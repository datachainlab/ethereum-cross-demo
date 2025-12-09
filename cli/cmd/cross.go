package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	clienttypes "github.com/cosmos/ibc-go/modules/core/02-client/types"
	authtypes "github.com/datachainlab/cross/x/core/auth/types"
	"github.com/datachainlab/cross/x/core/initiator/types"
	txtypes "github.com/datachainlab/cross/x/core/tx/types"
	xcctypes "github.com/datachainlab/cross/x/core/xcc/types"
	"github.com/datachainlab/fabric-besu-cross-demo/cmds/erc20/config"
	"github.com/datachainlab/fabric-besu-cross-demo/cmds/erc20/cross"
	"github.com/datachainlab/fabric-besu-cross-demo/cmds/erc20/cross/contract"
	besuauthtypes "github.com/datachainlab/fabric-besu-cross-demo/cmds/erc20/types"
	exttypes "github.com/datachainlab/fabric-besu-cross-demo/cmds/erc20/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func crossCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cross",
		Aliases: []string{"cross"},
		Short:   "manage cross tx",
	}

	cmd.AddCommand(
		getAddressCrossCmd(ctx),
		createContractTransactionCrossCmd(ctx),
		callInfoCrossCmd(ctx),
		createInitiateTxCmd(ctx),
		NewSendInitiateTxCmd(ctx),
	)

	return cmd
}

func setupCrossCMD(ctx *config.Context) (cross.CrossCMD, error) {
	cmdCfg := ctx.Config
	conn, err := cross.Connect(cmdCfg.BlockchainHost)
	if err != nil {
		return nil, err
	}

	token, err := contract.NewCrosssimplemodule(common.HexToAddress(cmdCfg.CrossModuleAddress), conn)
	if err != nil {
		return nil, err
	}

	pvtKey, err := cmdCfg.PrivateKey()
	if err != nil {
		return nil, err
	}

	return cross.NewCrossCMDImpl(conn, cmdCfg.ChainID, pvtKey, token), nil
}

func createInitiateTxCmd(ctx *config.Context) *cobra.Command {
	const (
		flagContractTransactions = "contract-txs"
	)

	cmd := &cobra.Command{
		Use:   "create-initiate-tx",
		Short: "Create a NewMsgInitiateTx transaction for a simple commit",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := cmd.Flags().GetStringSlice(flagContractTransactions)
			if err != nil {
				return err
			}

			ctxs, err := readContractTransactions(paths, ctx.Codec.UnmarshalJSON)
			if err != nil {
				return err
			}

			chainIDStr := fmt.Sprintf("%d", ctx.Config.ChainID)

			msg := types.NewMsgInitiateTx(
				nil,
				chainIDStr,

				uint64(time.Now().Unix()),
				txtypes.COMMIT_PROTOCOL_SIMPLE,
				ctxs,
				// TODO: Fix to ZeroHeight but currently scenario is failure.
				clienttypes.NewHeight(0, 10000),
				0,
			)

			// prepare output document
			closeFunc, err := setOutputFile(cmd)
			if err != nil {
				return err
			}
			defer closeFunc()

			bz, err := ctx.Codec.MarshalJSON(msg)
			if err != nil {
				return err
			}

			if _, err := cmd.OutOrStdout().Write(bz); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringSlice(flagContractTransactions, nil, "A file path to includes a contract transaction")
	_ = cmd.MarkFlagRequired(flagContractTransactions)

	cmd.Flags().String(flags.FlagOutputDocument, "", "The document will be written to the given file instead of STDOUT")

	return cmd
}

func NewSendInitiateTxCmd(ctx *config.Context) *cobra.Command {
	const (
		flagInitiateTx = "initiate-tx"
	)

	cmd := &cobra.Command{
		Use:   "send-initiate-tx",
		Short: "Send a NewMsgInitiateTx transaction for a simple commit",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := setupCrossCMD(ctx)
			if err != nil {
				return err
			}

			txPath, err := cmd.Flags().GetString(flagInitiateTx)
			if err != nil {
				return err
			}
			msg, err := readInitiateTx(ctx.Codec, txPath)
			if err != nil {
				return err
			}

			ethSignKey, err := cmd.Flags().GetString(FlagEthSignKey)
			if err != nil {
				return err
			}
			signer, err := getSigner(ctx, ethSignKey)
			if err != nil {
				return err
			}

			msg.Signers = []authtypes.Account{signer}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			// Convert types.MsgInitiateTx to contract.MsgInitiateTxData
			contractMsg := toContractMsgInitiateTxData(msg)

			// Debug: Log the contract message data
			bz, _ := json.MarshalIndent(contractMsg, "", "  ")
			log.Printf("Submitting InitiateTx with payload:\n%s", string(bz))

			// Submit the transaction to the contract via CrossCMD
			tx, err := cc.InitiateTx(contractMsg)
			if err != nil {
				return err
			}
			log.Printf("anvil.SendMsgs.result: tx='%v'", tx.Hash().Hex())

			// --- Debugging: Wait for Receipt and Check Status ---
			conn, err := cross.Connect(ctx.Config.BlockchainHost)
			if err != nil {
				return errors.Wrap(err, "failed to connect to backend for debugging")
			}

			log.Println("Waiting for transaction to be mined...")
			receipt, err := bind.WaitMined(cmd.Context(), conn, tx)
			if err != nil {
				return errors.Wrap(err, "failed to wait for tx receipt")
			}

			if receipt.Status == ethtypes.ReceiptStatusFailed {
				log.Printf("Transaction failed (Status: 0).")
				log.Printf("Gas Used: %d / Limit: %d", receipt.GasUsed, tx.Gas())

				log.Println("Attempting to fetch revert reason...")
				reason, callErr := getRevertReason(cmd.Context(), conn, tx, receipt)
				if callErr != nil {
					log.Printf("Failed to retrieve revert reason: %v", callErr)
				} else {
					log.Printf("Revert Reason/Data: %s", reason)
				}
				return fmt.Errorf("transaction failed with revert")
			}

			log.Println("Transaction executed successfully!")
			return nil
		},
	}

	cmd.Flags().String(flagInitiateTx, "", "A file path to includes a initiate-tx")
	_ = cmd.MarkFlagRequired(flagInitiateTx)

	cmd.Flags().String(flags.FlagOutputDocument, "", "The document will be written to the given file instead of STDOUT")

	ethSignKeyFlag(cmd)

	return cmd
}

// getRevertReason attempts to retrieve the revert reason of a failed transaction
func getRevertReason(ctx context.Context, conn *ethclient.Client, tx *ethtypes.Transaction, receipt *ethtypes.Receipt) (string, error) {
	// Reconstruct the message from the transaction
	from, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return "", errors.Wrap(err, "failed to recover sender")
	}

	callMsg := ethereum.CallMsg{
		From:     from,
		To:       tx.To(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasPrice(),
		Value:    tx.Value(),
		Data:     tx.Data(),
	}

	// CallContract at the block where the tx was included to simulate execution
	_, err = conn.CallContract(ctx, callMsg, receipt.BlockNumber)
	if err != nil {
		// Try to extract data from DataError
		var dataErr rpc.DataError
		if errors.As(err, &dataErr) {
			data := dataErr.ErrorData()
			if dataStr, ok := data.(string); ok {
				return parseRevertData(dataStr), nil
			}
			return fmt.Sprintf("DataError with unexpected type: %v", data), nil
		}

		// Some clients return the revert data in the error message string itself
		// Format usually "execution reverted: 0x..." or just "execution reverted"
		errMsg := err.Error()
		if strings.Contains(errMsg, "0x") {
			// Naive extraction attempt
			parts := strings.Split(errMsg, "0x")
			if len(parts) > 1 {
				// Take the last part which likely contains the hex data
				hexData := "0x" + parts[len(parts)-1]
				// Trim any non-hex chars if necessary (though usually it's clean or space separated)
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
		return fmt.Sprintf("Raw Data (Hex): %s (Decode Error: %v)", dataStr, err)
	}

	if len(dataBytes) < 4 {
		return fmt.Sprintf("Raw Data (Too short): %s", dataStr)
	}

	selector := dataBytes[:4]
	selectorHex := hex.EncodeToString(selector)

	// Check for standard Error(string) selector: 0x08c379a0
	if selectorHex == "08c379a0" {
		var reason string
		unpacker, _ := abi.NewType("string", "", nil)
		args := abi.Arguments{{Type: unpacker}}
		unpacked, err := args.Unpack(dataBytes[4:])
		if err == nil && len(unpacked) > 0 {
			reason = unpacked[0].(string)
			return fmt.Sprintf("Standard Error: %s", reason)
		}
	}

	// Check for Panic(uint256) selector: 0x4e487b71
	if selectorHex == "4e487b71" {
		var code *big.Int
		unpacker, _ := abi.NewType("uint256", "", nil)
		args := abi.Arguments{{Type: unpacker}}
		unpacked, err := args.Unpack(dataBytes[4:])
		if err == nil && len(unpacked) > 0 {
			code = unpacked[0].(*big.Int)
			return fmt.Sprintf("Panic Code: %s", code.String())
		}
	}

	// Custom Error
	return fmt.Sprintf("Custom Error Selector: 0x%s, Raw Data: %s", selectorHex, dataStr)
}

func ethSignKeyFlag(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().String(FlagEthSignKey, "", "the Ethereum Chain private key used by the importer for signing")
	return cmd
}

func readInitiateTx(m codec.JSONCodec, path string) (*types.MsgInitiateTx, error) {
	bz, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var iTx types.MsgInitiateTx
	if err := m.UnmarshalJSON(bz, &iTx); err != nil {
		return nil, err
	}

	return &iTx, nil
}

func getSigner(ctx *config.Context, ethSignKey string) (authtypes.Account, error) {
	var signer authtypes.Account
	addr, err := hexToEthereumAddress(ethSignKey)
	if err != nil {
		return authtypes.Account{}, err
	}
	signer = authtypes.Account{
		Id:       addr,
		AuthType: authtypes.NewAuthTypeExtension(&besuauthtypes.BesuAuthExtension{}),
	}
	return signer, nil
}

func hexToEthereumAddress(hexString string) ([]byte, error) {
	hexString = strings.TrimPrefix(hexString, "0x")
	if privKey, err := crypto.HexToECDSA(hexString); err != nil {
		return nil, err
	} else {
		return crypto.PubkeyToAddress(privKey.PublicKey).Bytes(), nil
	}
}

func hexToSecp256k1PrivKey(hexString string) (*secp256k1.PrivKey, error) {
	hexString = strings.TrimPrefix(hexString, "0x")
	bz, err := hex.DecodeString(hexString)
	if err != nil {
		return nil, err
	}

	return hd.Secp256k1.Generate()(bz).(*secp256k1.PrivKey), nil
}

func readContractTransactions(
	pathList []string,
	unmarshal func([]byte, proto.Message) error,
) ([]types.ContractTransaction, error) {
	cTxs := make([]types.ContractTransaction, 0, len(pathList))

	for _, path := range pathList {
		bz, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, err
		}

		var cTx types.ContractTransaction
		if err := unmarshal(bz, &cTx); err != nil {
			return nil, err
		}

		cTxs = append(cTxs, cTx)
	}

	return cTxs, nil
}

func getAddressCrossCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "address",
		Short: "Get Cross contract address",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(ctx.Config.CrossModuleAddress)
			return nil
		},
	}

	return cmd
}

func createContractTransactionCrossCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-contract-tx",
		Short: "Create a new contract transaction",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {

			signer, err := cmd.Flags().GetString(FlagSigner)
			if err != nil {
				return err
			}

			account := authtypes.Account{
				Id:       common.HexToAddress(signer).Bytes(),
				AuthType: authtypes.NewAuthTypeExtension(&exttypes.BesuAuthExtension{}),
			}

			callInfo, err := cmd.Flags().GetString(FlagCallInfo)
			if err != nil {
				return err
			}
			callInfoBytes, err := hex.DecodeString(callInfo)
			if err != nil {
				return err
			}

			initiatorChannel, err := cmd.Flags().GetString(FlagInitiatorChainChannel)
			if err != nil {
				return err
			}
			ci, err := parseChannelInfoFromString(initiatorChannel)
			if err != nil {
				return err
			}
			xcc, err := xcctypes.PackCrossChainChannel(ci)
			if err != nil {
				return err
			}

			cTx := types.ContractTransaction{
				CrossChainChannel: xcc,
				Signers:           []authtypes.Account{account},
				CallInfo:          callInfoBytes,
			}
			// prepare output document
			closeFunc, err := setOutputFile(cmd)
			if err != nil {
				return err
			}
			defer closeFunc()

			bz, err := ctx.Codec.MarshalJSON(&cTx)
			if err != nil {
				return err
			}

			if _, err := cmd.OutOrStdout().Write(bz); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().String(FlagInitiatorChainChannel, "", "The channel info: '<channelID>:<portID>'")
	_ = cmd.MarkFlagRequired(FlagInitiatorChainChannel)
	cmd.Flags().String(FlagSigner, "", "signer")
	_ = cmd.MarkFlagRequired(FlagSigner)
	cmd.Flags().String(FlagCallInfo, "", "call info")
	_ = cmd.MarkFlagRequired(FlagCallInfo)
	cmd.Flags().String(flags.FlagOutputDocument, "", "The document will be written to the given file instead of STDOUT")

	return cmd
}

func callInfoCrossCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call-info",
		Short: "Create a call info for contract transaction",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			contract, err := cmd.Flags().GetString(FlagAddress)
			if err != nil {
				return err
			}
			funcName, err := cmd.Flags().GetString(FlagFunctionName)
			if err != nil {
				return err
			}
			arguments, err := cmd.Flags().GetStringArray(FlagArguments)
			if err != nil {
				return err
			}
			argumentTypes, err := cmd.Flags().GetStringArray(FlagArgumentTypes)
			if err != nil {
				return err
			}
			if len(arguments) != len(argumentTypes) {
				return errors.New("arguments and argumentTypes are different length")
			}

			var convertedArgs []interface{}
			for i, a := range arguments {
				if a != "" {
					t := argumentTypes[i]
					arg, err := toArg(a, t)
					if err != nil {
						return err
					}
					convertedArgs = append(convertedArgs, arg)
				}
			}

			callInfo, err := toCallInfo(common.HexToAddress(contract), funcName, convertedArgs)
			if err != nil {
				return err
			}

			fmt.Println(hex.EncodeToString(callInfo))

			return nil
		},
	}

	cmd.Flags().String(FlagAddress, "", "contract address")
	_ = cmd.MarkFlagRequired(FlagAddress)
	cmd.Flags().String(FlagFunctionName, "", "function name")
	_ = cmd.MarkFlagRequired(FlagFunctionName)
	cmd.Flags().StringArray(FlagArguments, []string{""}, "arguments")
	cmd.Flags().StringArray(FlagArgumentTypes, []string{""}, "argument type")
	return cmd
}

func parseChannelInfoFromString(s string) (*xcctypes.ChannelInfo, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil, errors.New("channel format must be follow a format: '<channelID>:<portID>'")
	}
	return &xcctypes.ChannelInfo{Channel: parts[0], Port: parts[1]}, nil
}

func setOutputFile(cmd *cobra.Command) (func(), error) {
	outputDoc, err := cmd.Flags().GetString(flags.FlagOutputDocument)
	if err != nil {
		return func() {}, err
	}
	if outputDoc == "" {
		cmd.SetOut(cmd.OutOrStdout())
		return func() {}, nil
	}

	dir := filepath.Dir(outputDoc)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, fs.ModePerm); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	fp, err := os.OpenFile(outputDoc, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return func() {}, err
	}

	cmd.SetOut(fp)

	return func() { fp.Close() }, nil
}

type callInfo struct {
	Contract     common.Address
	FunctionName string
	Args         []interface{}
}

func toCallInfo(contract common.Address, funcName string, args []interface{}) ([]byte, error) {
	ci := callInfo{
		Contract:     contract,
		FunctionName: funcName,
		Args:         args,
	}
	return rlp.EncodeToBytes(ci)
}

func toArg(raw string, types string) (interface{}, error) {
	switch types {
	case "string":
		return raw, nil
	case "address":
		return common.HexToAddress(raw), nil
	case "int":
		i, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return nil, errors.New("failed to convert")
		}
		return i, nil
	default:
		return nil, errors.New("invalid args")
	}
}

func toContractAny(a *codectypes.Any) contract.GoogleProtobufAnyData {
	if a == nil {
		return contract.GoogleProtobufAnyData{}
	}
	return contract.GoogleProtobufAnyData{
		TypeUrl: a.TypeUrl,
		Value:   a.Value,
	}
}

func toContractMsgInitiateTxData(msg *types.MsgInitiateTx) contract.MsgInitiateTxData {
	// 1. Convert Signers
	var contractSigners []contract.AccountData
	for _, s := range msg.Signers {
		contractSigners = append(contractSigners, contract.AccountData{
			Id: s.Id,
			AuthType: contract.AuthTypeData{
				Mode:   uint8(s.AuthType.Mode),
				Option: toContractAny(s.AuthType.Option),
			},
		})
	}

	// 2. Convert ContractTransactions
	var contractTxs []contract.ContractTransactionData
	for _, ctx := range msg.ContractTransactions {
		var ctxSigners []contract.AccountData
		for _, s := range ctx.Signers {
			ctxSigners = append(ctxSigners, contract.AccountData{
				Id: s.Id,
				AuthType: contract.AuthTypeData{
					Mode:   uint8(s.AuthType.Mode),
					Option: toContractAny(s.AuthType.Option),
				},
			})
		}
		contractTxs = append(contractTxs, contract.ContractTransactionData{
			CrossChainChannel: toContractAny(ctx.CrossChainChannel),
			Signers:           ctxSigners,
			CallInfo:          ctx.CallInfo,
		})
	}

	// 3. Create contract message struct
	return contract.MsgInitiateTxData{
		ChainId:              msg.ChainId,
		Nonce:                msg.Nonce,
		CommitProtocol:       uint8(msg.CommitProtocol),
		ContractTransactions: contractTxs,
		Signers:              contractSigners,
		TimeoutHeight: contract.IbcCoreClientV1HeightData{
			RevisionNumber: msg.TimeoutHeight.RevisionNumber,
			RevisionHeight: msg.TimeoutHeight.RevisionHeight,
		},
		TimeoutTimestamp: msg.TimeoutTimestamp,
	}
}
