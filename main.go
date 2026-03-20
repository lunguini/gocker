package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lunguini/gocker/cmd"
)

func main() {
	app := cmd.NewApp()
	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
