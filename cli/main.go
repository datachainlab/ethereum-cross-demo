package main

import (
	"fmt"
	"os"

	"github.com/datachainlab/anvil-cross-demo/cmds/erc20/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}
