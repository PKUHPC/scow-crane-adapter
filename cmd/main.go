package main

import (
	"github.com/spf13/cobra"
	"scow-crane-adapter/cmd/app"
)

func main() {
	rootCmd := app.NewAdapterCommand()
	if err := rootCmd.Execute(); err != nil {
		cobra.CheckErr(err)
	}
}
