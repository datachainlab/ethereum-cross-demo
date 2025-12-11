package cmd

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cosmostypes "github.com/cosmos/cosmos-sdk/codec/types"
	cross "github.com/datachainlab/cross/x/core"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/config"
	exttypes "github.com/datachainlab/anvil-cross-demo/cmds/erc20/types"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "erc20cli",
	Short: "Operate erc20 contract",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")

	interfaceRegistry := cosmostypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(interfaceRegistry)
	cross.AppModuleBasic{}.RegisterInterfaces(interfaceRegistry)
	exttypes.RegisterInterfaces(interfaceRegistry)

	var cmdCfg config.Config
	ctx := &config.Context{Config: &cmdCfg, Codec: cdc}

	rootCmd.AddCommand(
		crossCmd(ctx),
		erc20Cmd(ctx),
	)

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		return initConfig(ctx)
	}
}

func Execute() error {
	return rootCmd.Execute()
}

func initConfig(ctx *config.Context) error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := homedir.Dir()
		if err != nil {
			return errors.Wrap(err, "Failed to change dir")
		}

		viper.AddConfigPath(home)
		viper.SetConfigName(".anvil-cross-demo")
	}

	viper.AutomaticEnv() // read in environment variables that match

	if err := viper.ReadInConfig(); err != nil {
		return errors.Wrap(err, "Failed to read config file")
	}

	if err := viper.Unmarshal(ctx.Config); err != nil {
		return errors.Wrap(err, "Failed to unmarshal")
	}
	return nil
}
