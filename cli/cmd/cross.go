package cmd

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	clienttypes "github.com/cosmos/ibc-go/modules/core/02-client/types"
	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/config"
	contract "github.com/datachainlab/anvil-cross-demo/cmds/erc20/contract/crosssimplemodule"
	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/cross"
	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/eth"
	extauthtypes "github.com/datachainlab/anvil-cross-demo/cmds/erc20/types"
	authtypes "github.com/datachainlab/cross/x/core/auth/types"
	"github.com/datachainlab/cross/x/core/initiator/types"
	txtypes "github.com/datachainlab/cross/x/core/tx/types"
	xcctypes "github.com/datachainlab/cross/x/core/xcc/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// CrossCmd returns the root command for cross-chain operations.
func crossCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cross",
		Aliases: []string{"cross"},
		Short:   "Manage cross-chain transactions",
	}

	cmd.AddCommand(
		newAddressCmd(ctx),
		newCreateContractTxCmd(ctx),
		newCallInfoCmd(ctx),
		newCreateInitiateTxCmd(ctx),
		newSendInitiateTxCmd(ctx),
		newSendExecuteTxCmd(ctx),
		newTxAuthStateCmd(ctx),
		newExtSignTxCmd(ctx),
		newCoordinatorStateCmd(ctx),
	)

	return cmd
}

func setupCrossClient(ctx *config.Context) (cross.CrossCMD, error) {
	conn, err := eth.Connect(ctx.Config.BlockchainHost)
	if err != nil {
		return nil, err
	}

	crossAddr := common.HexToAddress(ctx.Config.CrossModuleAddress)
	token, err := contract.NewCrosssimplemodule(crossAddr, conn)
	if err != nil {
		return nil, err
	}

	pvtKey, err := ctx.Config.PrivateKey()
	if err != nil {
		return nil, err
	}

	return cross.NewCrossCMDImpl(conn, ctx.Config.ChainID, pvtKey, token, crossAddr), nil
}

func newCoordinatorStateCmd(ctx *config.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "coordinator-state [tx-id]",
		Short: "Query the coordinator state of a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := setupCrossClient(ctx)
			if err != nil {
				return err
			}

			txID, err := decodeHexParam(args[0])
			if err != nil {
				return fmt.Errorf("invalid tx-id: %w", err)
			}

			req := contract.QueryCoordinatorStateRequestData{
				TxId: txID,
			}

			resp, err := client.QueryCoordinatorState(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to query coordinator state: %w", err)
			}

			return printJSON(resp)
		},
	}
}

func newExtSignTxCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ext-sign-tx [tx-id]",
		Short: "Sign a cross-chain transaction using Extension Auth",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := setupCrossClient(ctx)
			if err != nil {
				return err
			}

			txIDBytes, err := decodeHexParam(args[0])
			if err != nil || len(txIDBytes) != 32 {
				return fmt.Errorf("invalid tx-id: must be 32 bytes hex: %w", err)
			}
			var txID [32]byte
			copy(txID[:], txIDBytes)

			ethSignKey, err := cmd.Flags().GetString(FlagEthSignKey)
			if err != nil {
				return err
			}

			ethPrivKey, err := crypto.HexToECDSA(strings.TrimPrefix(ethSignKey, "0x"))
			if err != nil {
				return fmt.Errorf("failed to parse private key: %w", err)
			}
			myAddr := crypto.PubkeyToAddress(ethPrivKey.PublicKey)

			log.Printf("Signer Address: %s", myAddr.Hex())

			// 1. Generate signature
			// Keccak256("\x19Ethereum Signed Message:\n32" + signMSG)
			msgHash := crypto.Keccak256Hash(
				[]byte("\x19Ethereum Signed Message:\n32"),
				txID[:],
			)

			signature, err := crypto.Sign(msgHash.Bytes(), ethPrivKey)
			if err != nil {
				return fmt.Errorf("failed to sign message: %w", err)
			}
			// Adjust V for EIP-155 / Solidity ECDSA compatibility
			if signature[64] < 27 {
				signature[64] += 27
			}

			// 2. ABI Encode: (bytes signature, bytes32 signMSG)
			bytesType, _ := abi.NewType("bytes", "", nil)
			bytes32Type, _ := abi.NewType("bytes32", "", nil)
			arguments := abi.Arguments{
				{Type: bytesType},
				{Type: bytes32Type},
			}

			packedValue, err := arguments.Pack(signature, txID)
			if err != nil {
				return fmt.Errorf("failed to abi encode: %w", err)
			}

			// 3. Construct MsgExtSignTxData
			inputMsg := contract.MsgExtSignTxData{
				TxID: txID[:],
				Signers: []contract.AccountData{
					{
						Id: myAddr.Bytes(),
						AuthType: contract.AuthTypeData{
							Mode: 3, // AUTH_MODE_EXTENSION
							Option: contract.GoogleProtobufAnyData{
								TypeUrl: "/extension.types.SampleAuthExtension",
								Value:   packedValue,
							},
						},
					},
				},
			}

			tx, err := client.ExtSignTx(inputMsg)
			if err != nil {
				return fmt.Errorf("failed to execute ExtSignTx: %w", err)
			}

			log.Printf("ExtSignTx submitted successfully! Tx Hash: %s\n", tx.Hash().Hex())
			return nil
		},
	}

	cmd.Flags().String(FlagEthSignKey, "", "Ethereum private key for signing")
	_ = cmd.MarkFlagRequired(FlagEthSignKey)

	return cmd
}

func newTxAuthStateCmd(ctx *config.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "tx-auth-state [tx-id]",
		Short: "Query the authentication state of a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := setupCrossClient(ctx)
			if err != nil {
				return err
			}

			txID, err := decodeHexParam(args[0])
			if err != nil {
				return fmt.Errorf("invalid tx-id: %w", err)
			}

			req := contract.QueryTxAuthStateRequestData{
				TxID: txID,
			}

			resp, err := client.QueryTxAuthState(cmd.Context(), req)
			if err != nil {
				return err
			}

			return printJSON(resp)
		},
	}
}

func newCreateInitiateTxCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-initiate-tx",
		Short: "Create a NewMsgInitiateTx transaction for a simple commit",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := cmd.Flags().GetStringSlice(FlagContractTransactions)
			if err != nil {
				return err
			}

			cTxs, err := readContractTransactions(paths, ctx.Codec.UnmarshalJSON)
			if err != nil {
				return err
			}

			msg := types.NewMsgInitiateTx(
				nil,
				fmt.Sprintf("%d", ctx.Config.ChainID),
				uint64(time.Now().Unix()),
				txtypes.COMMIT_PROTOCOL_SIMPLE,
				cTxs,
				clienttypes.NewHeight(0, 10000), // Fixed height for demo
				0,
			)

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

	cmd.Flags().StringSlice(FlagContractTransactions, nil, "File paths to contract transactions")
	_ = cmd.MarkFlagRequired(FlagContractTransactions)
	cmd.Flags().String(flags.FlagOutputDocument, "", "Write output to file instead of STDOUT")

	return cmd
}

func newSendInitiateTxCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-initiate-tx",
		Short: "Send a NewMsgInitiateTx transaction for a simple commit",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := setupCrossClient(ctx)
			if err != nil {
				return err
			}

			txPath, err := cmd.Flags().GetString(FlagInitiateTx)
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
			signer, err := getSigner(ethSignKey)
			if err != nil {
				return err
			}

			msg.Signers = []authtypes.Account{signer}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			contractMsg := toContractMsgInitiateTxData(msg)

			// Submit via CrossCMD
			tx, err := client.InitiateTx(contractMsg)
			if err != nil {
				return err
			}
			log.Printf("Transaction submitted: hash='%v'", tx.Hash().Hex())
			log.Println("Waiting for transaction to be mined...")

			// Wait for mining and event emission
			event, err := client.GetTxInitiatedEvent(cmd.Context(), tx)
			if err != nil {
				return err
			}

			log.Println("Transaction executed successfully!")
			fmt.Printf("\n=== InitiateTx Result ===\n")
			fmt.Printf("TxID (Hex): 0x%x\n", event.TxID)
			fmt.Printf("Sender:     %s\n", event.Proposer.Hex())
			fmt.Printf("=========================\n")

			return nil
		},
	}

	cmd.Flags().String(FlagInitiateTx, "", "File path to initiate-tx")
	_ = cmd.MarkFlagRequired(FlagInitiateTx)
	cmd.Flags().String(flags.FlagOutputDocument, "", "Write output to file instead of STDOUT")
	cmd.Flags().String(FlagEthSignKey, "", "Ethereum private key for signing")

	return cmd
}

func newSendExecuteTxCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-execute-tx",
		Short: "Send an ExecuteTx transaction for a simple commit",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := setupCrossClient(ctx)
			if err != nil {
				return err
			}

			txPath, err := cmd.Flags().GetString(FlagInitiateTx)
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
			signer, err := getSigner(ethSignKey)
			if err != nil {
				return err
			}

			msg.Signers = []authtypes.Account{signer}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			contractMsg := toContractMsgInitiateTxData(msg)

			// Submit via CrossCMD
			tx, err := client.ExecuteTx(contractMsg)
			if err != nil {
				return err
			}
			log.Printf("Transaction submitted: hash='%v'", tx.Hash().Hex())
			log.Println("Waiting for transaction to be mined...")

			// Wait for mining and event emission
			event, err := client.GetTxExecutedEvent(cmd.Context(), tx)
			if err != nil {
				return err
			}

			log.Println("Transaction executed successfully!")
			log.Printf("\n=== ExecuteTx Result ===\n")
			log.Printf("TxID (Hex): 0x%x\n", event.TxID)
			log.Printf("Sender:   %s\n", event.Proposer.Hex())
			log.Printf("=========================\n")

			return nil
		},
	}

	cmd.Flags().String(FlagInitiateTx, "", "File path to execute-tx")
	_ = cmd.MarkFlagRequired(FlagInitiateTx)
	cmd.Flags().String(flags.FlagOutputDocument, "", "Write output to file instead of STDOUT")
	cmd.Flags().String(FlagEthSignKey, "", "Ethereum private key for signing")
	return cmd
}

func newAddressCmd(ctx *config.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "address",
		Short: "Get Cross contract address",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(ctx.Config.CrossModuleAddress)
			return nil
		},
	}
}

func newCreateContractTxCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-contract-tx",
		Short: "Create a new contract transaction",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			signerStr, err := cmd.Flags().GetString(FlagSigner)
			if err != nil {
				return err
			}

			account := authtypes.Account{
				Id:       common.HexToAddress(signerStr).Bytes(),
				AuthType: authtypes.NewAuthTypeExtension(&extauthtypes.SampleAuthExtension{}),
			}

			callInfoHex, err := cmd.Flags().GetString(FlagCallInfo)
			if err != nil {
				return err
			}
			callInfoBytes, err := hex.DecodeString(callInfoHex)
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

	cmd.Flags().String(FlagInitiatorChainChannel, "", "Channel info: '<channelID>:<portID>'")
	_ = cmd.MarkFlagRequired(FlagInitiatorChainChannel)
	cmd.Flags().String(FlagSigner, "", "Signer address")
	_ = cmd.MarkFlagRequired(FlagSigner)
	cmd.Flags().String(FlagCallInfo, "", "Call info hex")
	_ = cmd.MarkFlagRequired(FlagCallInfo)
	cmd.Flags().String(flags.FlagOutputDocument, "", "Write output to file instead of STDOUT")

	return cmd
}

func newCallInfoCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call-info",
		Short: "Create a call info for contract transaction",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			contractAddr, err := cmd.Flags().GetString(FlagAddress)
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
				return errors.New("arguments and argumentTypes length mismatch")
			}

			var convertedArgs []interface{}
			for i, a := range arguments {
				if a != "" {
					arg, err := toArg(a, argumentTypes[i])
					if err != nil {
						return err
					}
					convertedArgs = append(convertedArgs, arg)
				}
			}

			callInfoBytes, err := toCallInfo(common.HexToAddress(contractAddr), funcName, convertedArgs)
			if err != nil {
				return err
			}

			fmt.Println(hex.EncodeToString(callInfoBytes))
			return nil
		},
	}

	cmd.Flags().String(FlagAddress, "", "Contract address")
	_ = cmd.MarkFlagRequired(FlagAddress)
	cmd.Flags().String(FlagFunctionName, "", "Function name")
	_ = cmd.MarkFlagRequired(FlagFunctionName)
	cmd.Flags().StringArray(FlagArguments, []string{""}, "Arguments")
	cmd.Flags().StringArray(FlagArgumentTypes, []string{""}, "Argument types")
	return cmd
}

// --- Helpers ---

func decodeHexParam(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}

func printJSON(v interface{}) error {
	bz, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to json: %w", err)
	}
	log.Println(string(bz))
	return nil
}

func readInitiateTx(m codec.JSONCodec, path string) (*types.MsgInitiateTx, error) {
	bz, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var iTx types.MsgInitiateTx
	if err := m.UnmarshalJSON(bz, &iTx); err != nil {
		return nil, err
	}
	return &iTx, nil
}

func getSigner(ethSignKey string) (authtypes.Account, error) {
	hexString := strings.TrimPrefix(ethSignKey, "0x")
	privKey, err := crypto.HexToECDSA(hexString)
	if err != nil {
		return authtypes.Account{}, err
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey).Bytes()

	return authtypes.Account{
		Id:       addr,
		AuthType: authtypes.NewAuthTypeExtension(&extauthtypes.SampleAuthExtension{}),
	}, nil
}

func readContractTransactions(pathList []string, unmarshal func([]byte, proto.Message) error) ([]types.ContractTransaction, error) {
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

func parseChannelInfoFromString(s string) (*xcctypes.ChannelInfo, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil, errors.New("invalid channel format: expected '<channelID>:<portID>'")
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

// --- Data Conversion & RLP ---

type callInfoStruct struct {
	Contract     common.Address
	FunctionName string
	Args         []interface{}
}

func toCallInfo(contractAddr common.Address, funcName string, args []interface{}) ([]byte, error) {
	ci := callInfoStruct{
		Contract:     contractAddr,
		FunctionName: funcName,
		Args:         args,
	}
	return rlp.EncodeToBytes(ci)
}

func toArg(raw string, typeName string) (interface{}, error) {
	switch typeName {
	case "string":
		return raw, nil
	case "address":
		return common.HexToAddress(raw), nil
	case "int":
		i, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return nil, errors.New("failed to convert int")
		}
		return i, nil
	default:
		return nil, fmt.Errorf("invalid arg type: %s", typeName)
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
	// Convert Signers
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

	// Convert ContractTransactions
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
