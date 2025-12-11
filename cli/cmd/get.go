package cmd

import (
	"context"
	"fmt"
	"math/big"

	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/erc20"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/config"
)

func getCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get some info",
	}

	cmd.AddCommand(
		blockNumberGetCmd(ctx),
		eventGetCmd(ctx),
	)
	return cmd
}

func blockNumberGetCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block",
		Short: "Get block number",
		RunE: func(cmd *cobra.Command, args []string) error {
			block, err := blockNumber(ctx)
			if err != nil {
				return err
			}
			fmt.Println(block)
			return nil
		},
	}

	return cmd
}

func blockNumber(ctx *config.Context) (uint64, error) {
	cmdCfg := ctx.Config
	conn, err := erc20.Connect(cmdCfg.BlockchainHost)
	if err != nil {
		return 0, errors.Wrap(err, "Failed to connect")
	}

	block, err := conn.BlockNumber(context.Background())
	if err != nil {
		return 0, errors.Wrap(err, "Failed to get")
	}
	return block, nil
}

func eventGetCmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Get event",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := setupClient(ctx)
			if err != nil {
				return err
			}

			fromBlock, err := cmd.Flags().GetString(FlagFromBlock)
			if err != nil {
				return err
			}
			i, ok := new(big.Int).SetString(fromBlock, 10)
			if !ok {
				return err
			}
			contractAddress, err := cmd.Flags().GetString(FlagContractAddress)
			if err != nil {
				return err
			}

			eventSignature, err := cmd.Flags().GetString(FlagEventSignature)
			if err != nil {
				return err
			}
			eventHash := crypto.Keccak256Hash([]byte(eventSignature))

			topics := [][]common.Hash{{eventHash}}
			indexes, err := cmd.Flags().GetStringArray(FlagIndexes)
			if err != nil {
				return err
			}
			indexTypes, err := cmd.Flags().GetStringArray(FlagIndexTypes)
			if err != nil {
				return err
			}
			if len(indexes) != len(indexTypes) {
				return errors.New("indexes and indexTypes are different length")
			}

			for idx, i := range indexes {
				if i != "" {
					t := indexTypes[idx]
					hash, err := toHash(i, t)
					if err != nil {
						return err
					}
					topics = append(topics, []common.Hash{hash})
				}
			}

			query := ethereum.FilterQuery{
				FromBlock: i,
				Addresses: []common.Address{
					common.HexToAddress(contractAddress),
				},
				Topics: topics,
			}
			logs, err := client.FilterLogs(context.Background(), query)
			if err != nil {
				return err
			}

			if len(logs) == 0 {
				return errors.New("event not found")
			}
			fmt.Println(logs[len(logs)-1].TxHash.Hex())
			return nil
		},
	}

	cmd.Flags().StringP(FlagFromBlock, "f", "", "start block number")
	_ = cmd.MarkFlagRequired(FlagFromBlock)
	cmd.Flags().StringP(FlagEventSignature, "e", "", "event signature")
	_ = cmd.MarkFlagRequired(FlagEventSignature)
	cmd.Flags().StringP(FlagContractAddress, "c", "", "contract address")
	_ = cmd.MarkFlagRequired(FlagContractAddress)
	cmd.Flags().StringArray(FlagIndexes, []string{""}, "index")
	cmd.Flags().StringArray(FlagIndexTypes, []string{""}, "index type")

	return cmd
}

func setupClient(ctx *config.Context) (*ethclient.Client, error) {
	cmdCfg := ctx.Config
	client, err := erc20.Connect(cmdCfg.BlockchainHost)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func toHash(raw string, types string) (common.Hash, error) {
	switch types {
	case "string":
		return crypto.Keccak256Hash([]byte(raw)), nil
	case "address":
		return common.HexToHash(raw), nil
	case "int":
		i, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return common.Hash{}, errors.New("failed to convert")
		}
		return common.BigToHash(i), nil
	default:
		return common.Hash{}, errors.New("invalid args")
	}
}
