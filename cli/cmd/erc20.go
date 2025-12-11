package cmd

import (
	"fmt"
	"math/big"

	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/config"
	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/erc20"
	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/erc20/contract"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func erc20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "erc20",
		Short: "Operate erc20 token",
	}

	cmd.AddCommand(
		getAddressERC20Cmd(ctx),
		mintERC20Cmd(ctx),
		approveERC20Cmd(ctx),
		transferERC20Cmd(ctx),  // Added
		allowanceERC20Cmd(ctx), // Added
		balanceOfERC20Cmd(ctx),
	)
	return cmd
}

func setupERC20CMD(ctx *config.Context) (erc20.ERC20CMD, error) {
	cmdCfg := ctx.Config
	conn, err := erc20.Connect(cmdCfg.BlockchainHost)
	if err != nil {
		return nil, err
	}

	token, err := contract.NewMyerc20(common.HexToAddress(cmdCfg.ERC20TokenAddress), conn)
	if err != nil {
		return nil, err
	}

	pvtKey, err := cmdCfg.PrivateKey()
	if err != nil {
		return nil, err
	}

	return erc20.NewERC20CMDImpl(conn, cmdCfg.ChainID, pvtKey, token), nil
}

func getAddressERC20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "address",
		Short: "Get Contract address",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(ctx.Config.ERC20TokenAddress)
			return nil
		},
	}

	return cmd
}

func mintERC20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mint",
		Short: "Mint token",
		RunE: func(cmd *cobra.Command, args []string) error {
			erc20CMD, err := setupERC20CMD(ctx)
			if err != nil {
				return err
			}

			receiver, err := cmd.Flags().GetString(FlagAddress)
			if err != nil {
				return err
			}
			amountStr, err := cmd.Flags().GetString(FlagAmount)
			if err != nil {
				return err
			}
			amount, ok := new(big.Int).SetString(amountStr, 10)
			if !ok {
				return errors.New("failed to convert amount")
			}

			tx, err := erc20CMD.Mint(common.HexToAddress(receiver), amount)
			if err != nil {
				return err
			}

			printTx(tx)
			return nil
		},
	}

	cmd.Flags().StringP(FlagAddress, "a", "", "receiver address taken token")
	_ = cmd.MarkFlagRequired(FlagAddress)
	cmd.Flags().StringP(FlagAmount, "m", "", "mint amount")
	_ = cmd.MarkFlagRequired(FlagAmount)

	return cmd
}

func approveERC20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve token",
		RunE: func(cmd *cobra.Command, args []string) error {
			erc20CMD, err := setupERC20CMD(ctx)
			if err != nil {
				return err
			}

			spender, err := cmd.Flags().GetString(FlagAddress)
			if err != nil {
				return err
			}
			amountStr, err := cmd.Flags().GetString(FlagAmount)
			if err != nil {
				return err
			}
			amount, ok := new(big.Int).SetString(amountStr, 10)
			if !ok {
				return errors.New("failed to convert amount")
			}

			tx, err := erc20CMD.Approve(common.HexToAddress(spender), amount)
			if err != nil {
				return err
			}

			printTx(tx)
			return nil
		},
	}

	cmd.Flags().StringP(FlagAddress, "a", "", "spender address")
	_ = cmd.MarkFlagRequired(FlagAddress)
	cmd.Flags().StringP(FlagAmount, "m", "", "approve amount")
	_ = cmd.MarkFlagRequired(FlagAmount)

	return cmd
}

func transferERC20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Transfer token",
		RunE: func(cmd *cobra.Command, args []string) error {
			erc20CMD, err := setupERC20CMD(ctx)
			if err != nil {
				return err
			}

			to, err := cmd.Flags().GetString(FlagAddress)
			if err != nil {
				return err
			}
			amountStr, err := cmd.Flags().GetString(FlagAmount)
			if err != nil {
				return err
			}
			amount, ok := new(big.Int).SetString(amountStr, 10)
			if !ok {
				return errors.New("failed to convert amount")
			}

			tx, err := erc20CMD.Transfer(common.HexToAddress(to), amount)
			if err != nil {
				return err
			}

			printTx(tx)
			return nil
		},
	}

	cmd.Flags().StringP(FlagAddress, "a", "", "recipient address")
	_ = cmd.MarkFlagRequired(FlagAddress)
	cmd.Flags().StringP(FlagAmount, "m", "", "transfer amount")
	_ = cmd.MarkFlagRequired(FlagAmount)

	return cmd
}

func allowanceERC20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allowance",
		Short: "Get remaining allowance",
		RunE: func(cmd *cobra.Command, args []string) error {
			erc20CMD, err := setupERC20CMD(ctx)
			if err != nil {
				return err
			}

			owner, err := cmd.Flags().GetString(FlagOwner)
			if err != nil {
				return err
			}
			spender, err := cmd.Flags().GetString(FlagSpender)
			if err != nil {
				return err
			}

			result, err := erc20CMD.Allowance(common.HexToAddress(owner), common.HexToAddress(spender))
			if err != nil {
				return err
			}

			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().String(FlagOwner, "", "owner address")
	_ = cmd.MarkFlagRequired(FlagOwner)
	cmd.Flags().String(FlagSpender, "", "spender address")
	_ = cmd.MarkFlagRequired(FlagSpender)

	return cmd
}

func balanceOfERC20Cmd(ctx *config.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balanceOf",
		Short: "Get balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			erc20CMD, err := setupERC20CMD(ctx)
			if err != nil {
				return err
			}

			account, err := cmd.Flags().GetString(FlagAddress)
			if err != nil {
				return err
			}

			result, err := erc20CMD.BalanceOf(common.HexToAddress(account))
			if err != nil {
				return err
			}

			fmt.Println(result)
			return nil
		},
	}

	cmd.Flags().StringP(FlagAddress, "a", "", "address")
	_ = cmd.MarkFlagRequired(FlagAddress)

	return cmd
}

func printTx(tx *types.Transaction) {
	fmt.Println(tx.Hash().Hex())
}
