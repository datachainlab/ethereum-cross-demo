package cmd

import (
	"encoding/hex"
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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
			if err := ctx.Chain.Connect(); err != nil {
				return err
			}

			msg, err := readInitiateTx(ctx.Codec, viper.GetString(flagInitiateTx))
			if err != nil {
				return err
			}

			ethSignKey := viper.GetString(FlagEthSignKey)
			signer, err := getSigner(ctx, ethSignKey)
			if err != nil {
				return err
			}

			msg.Signers = []authtypes.Account{signer}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			var res []byte
			if privKey, err := hexToSecp256k1PrivKey(ethSignKey); err != nil {
				return err
			} else if txBytes, err := buildTx(ctx.Codec.InterfaceRegistry(), privKey, msg); err != nil {
				return err
			} else {
				res, err = ctx.Chain.Contract().SubmitTransaction("handleTx", string(txBytes))
				if err != nil {
					return err
				}
				log.Printf("fabric.SendMsgs.result: res='%v' err='%v'", res, err)
			}

			if err := ctx.Chain.OutputTxIDFromEvent(res); err != nil {
				return err
			}

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
	if privKey, err := crypto.HexToECDSA(hexString); err != nil {
		return nil, err
	} else {
		return crypto.PubkeyToAddress(privKey.PublicKey).Bytes(), nil
	}
}

func hexToSecp256k1PrivKey(hexString string) (*secp256k1.PrivKey, error) {
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
