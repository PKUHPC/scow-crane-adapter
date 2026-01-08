package main

import (
	"github.com/spf13/cobra"
	"scow-crane-adapter/cmd/app"
	"scow-crane-adapter/pkg/utils"
	"time"
)

func main() {
	utils.SetAdapterStartTime(time.Now())
	rootCmd := app.NewAdapterCommand()
	if err := rootCmd.Execute(); err != nil {
		cobra.CheckErr(err)
	}
}
