package cmd

import (
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
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
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
		txAuthStateCmd(ctx),
		extSignTxCmd(ctx),
		coordinatorStateCmd(ctx),
	)

	return cmd
}

func setupCrossCMD(ctx *config.Context) (cross.CrossCMD, error) {
	cmdCfg := ctx.Config
	conn, err := cross.Connect(cmdCfg.BlockchainHost)
	if err != nil {
		return nil, err
	}

	crossAddr := common.HexToAddress(cmdCfg.CrossModuleAddress)

	token, err := contract.NewCrosssimplemodule(crossAddr, conn)
	if err != nil {
		return nil, err
	}

	pvtKey, err := cmdCfg.PrivateKey()
	if err != nil {
		return nil, err
	}

	return cross.NewCrossCMDImpl(conn, cmdCfg.ChainID, pvtKey, token, crossAddr), nil
}

func coordinatorStateCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coordinator-state [tx-id]",
		Short: "Query the coordinator state of a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. CrossCMDのセットアップ
			cc, err := setupCrossCMD(ctx)
			if err != nil {
				return err
			}

			// 2. 引数(TxID)のパース
			// 0xプレフィックスを除去してデコード
			txIDStr := strings.TrimPrefix(args[0], "0x")
			txID, err := hex.DecodeString(txIDStr)
			if err != nil {
				return fmt.Errorf("invalid tx-id: must be hex string: %w", err)
			}

			// 3. リクエストデータの作成
			// contractパッケージの構造体定義を使用
			req := contract.QueryCoordinatorStateRequestData{
				TxId: txID,
			}

			// 4. 問い合わせ実行 (CrossCMD経由)
			resp, err := cc.QueryCoordinatorState(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to query coordinator state: %w", err)
			}

			// 5. 結果表示
			// Goの[]byteフィールドはJSON標準ではBase64文字列として出力されます。
			// 値を確認する際は echo "Base64Str" | base64 -d | xxd -p 等でデコードしてください。
			bz, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal response to json: %w", err)
			}
			log.Println(string(bz))

			return nil
		},
	}

	return cmd
}

func extSignTxCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ext-sign-tx [tx-id]",
		Short: "Sign a cross-chain transaction using Extension Auth",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. CrossCMDのセットアップ (接続、Contractインスタンス取得)
			cc, err := setupCrossCMD(ctx)
			if err != nil {
				return err
			}

			// 2. 引数(TxID)のパース
			txIDStr := strings.TrimPrefix(args[0], "0x")
			txIDBytes, err := hex.DecodeString(txIDStr)
			if err != nil || len(txIDBytes) != 32 {
				return fmt.Errorf("invalid tx-id: must be 32 bytes hex: %w", err)
			}
			var txID [32]byte
			copy(txID[:], txIDBytes)

			// 3. 署名用鍵の取得 (FlagEthSignKeyから)
			ethSignKey, err := cmd.Flags().GetString(FlagEthSignKey)
			if err != nil {
				return err
			}
			// 秘密鍵オブジェクトへ変換 (署名生成に使用)
			ethPrivKey, err := crypto.HexToECDSA(strings.TrimPrefix(ethSignKey, "0x"))
			if err != nil {
				return fmt.Errorf("failed to parse private key: %w", err)
			}
			// アドレス取得 (Signer ID用)
			myAddr := crypto.PubkeyToAddress(ethPrivKey.PublicKey)

			log.Printf("\n=== DEBUG INFO ===\n")
			log.Printf("Using Private Key: ...%s\n", ethSignKey[len(ethSignKey)-4:]) // 末尾のみ表示
			log.Printf("Derived Address:   %s\n", myAddr.Hex())
			log.Printf("Target TxID:       %x\n", txID)
			log.Printf("==================\n\n")

			// 4. SampleExtensionVerifier 向け署名データの作成
			// signMSG として TxID を使用
			signMSG := txID

			// Ethereum Signed Message Hash を計算
			// Keccak256("\x19Ethereum Signed Message:\n32" + signMSG)
			msgHash := crypto.Keccak256Hash(
				[]byte("\x19Ethereum Signed Message:\n32"),
				signMSG[:],
			)

			// 署名生成 (65 bytes: R|S|V)
			signature, err := crypto.Sign(msgHash.Bytes(), ethPrivKey)
			if err != nil {
				return fmt.Errorf("failed to sign message: %w", err)
			}
			// EIP-155 / Solidity ECDSA 互換のため V を 27/28 に調整
			if signature[64] < 27 {
				signature[64] += 27
			}

			// 5. ABIエンコーディング (bytes signature, bytes32 signMSG)
			bytesType, _ := abi.NewType("bytes", "", nil)
			bytes32Type, _ := abi.NewType("bytes32", "", nil)
			arguments := abi.Arguments{
				{Type: bytesType},
				{Type: bytes32Type},
			}

			packedValue, err := arguments.Pack(signature, signMSG)
			if err != nil {
				return fmt.Errorf("failed to abi encode: %w", err)
			}

			// 6. MsgExtSignTxData の構築
			// Note: contract.MsgExtSignTxData の定義は bind 生成物に依存しますが、
			// 一般的な abigen 出力を想定しています。
			inputMsg := contract.MsgExtSignTxData{
				TxID: txID[:], // bindingによっては [32]byte か []byte か異なります
				Signers: []contract.AccountData{
					{
						Id: myAddr.Bytes(),
						AuthType: contract.AuthTypeData{
							Mode: 3, // AUTH_MODE_EXTENSION
							Option: contract.GoogleProtobufAnyData{
								TypeUrl: "/verifier.sample.extension",
								Value:   packedValue,
							},
						},
					},
				},
			}

			// Debug : Log the input message
			bz, _ := json.MarshalIndent(inputMsg, "", "  ")
			log.Printf("Submitting ExtSignTx with payload:\n%s", string(bz))

			// 7. トランザクション送信
			tx, err := cc.ExtSignTx(inputMsg)
			if err != nil {
				return fmt.Errorf("failed to execute ExtSignTx: %w", err)
			}

			log.Printf("ExtSignTx submitted successfully! Tx Hash: %s\n", tx.Hash().Hex())
			return nil
		},
	}

	// 署名用の鍵フラグを追加
	ethSignKeyFlag(cmd)
	_ = cmd.MarkFlagRequired(FlagEthSignKey)

	return cmd
}

func txAuthStateCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx-auth-state [tx-id]",
		Short: "Query the authentication state of a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := setupCrossCMD(ctx)
			if err != nil {
				return err
			}

			// 引数(Hex文字列)を []byte に変換
			txIDStr := strings.TrimPrefix(args[0], "0x")
			txID, err := hex.DecodeString(txIDStr)
			if err != nil {
				return fmt.Errorf("invalid tx-id: %w", err)
			}

			// リクエスト作成
			req := contract.QueryTxAuthStateRequestData{
				TxID: txID,
			}

			// 問い合わせ実行
			resp, err := cc.QueryTxAuthState(cmd.Context(), req)
			if err != nil {
				return err
			}

			// 結果表示
			// Note: []byte型(Idなど)はJSONだとBase64になります。
			// 必要であればHexに変換する処理を挟んでください。今回はそのまま出します。
			bz, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return err
			}
			log.Println(string(bz))

			return nil
		},
	}

	return cmd
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
			log.Printf("Transaction submitted: hash='%v'", tx.Hash().Hex())
			log.Println("Waiting for transaction to be mined...")

			// 2. マイニング待ち & イベント取得 (エラー解析も内部で実施)
			event, err := cc.GetTxInitiatedEvent(cmd.Context(), tx)
			if err != nil {
				return err // 詳細なRevert理由などが含まれています
			}

			log.Println("Transaction executed successfully!")

			// 3. 結果表示
			fmt.Printf("\n=== InitiateTx Result ===\n")
			fmt.Printf("TxID (Hex): 0x%x\n", event.TxID)
			fmt.Printf("Sender:     %s\n", event.Proposer.Hex())
			fmt.Printf("=========================\n")

			return nil
		},
	}

	cmd.Flags().String(flagInitiateTx, "", "A file path to includes a initiate-tx")
	_ = cmd.MarkFlagRequired(flagInitiateTx)

	cmd.Flags().String(flags.FlagOutputDocument, "", "The document will be written to the given file instead of STDOUT")

	ethSignKeyFlag(cmd)

	return cmd
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
