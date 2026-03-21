package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lunguini/gocker/cmd"
)

var version = "dev"

func main() {
	app := cmd.NewApp(version)
	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
