package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/format"
	"github.com/urfave/cli/v3"
)

func newPsCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "ps",
		Usage: "List containers",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "Show all containers"},
			&cli.BoolFlag{Name: "quiet", Aliases: []string{"q"}, Usage: "Only display container IDs"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 0 {
				return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
			}
			containers, err := eng.ContainerList(ctx, cmd.Bool("all"))
			if err != nil {
				return err
			}

			if cmd.Bool("quiet") {
				for _, c := range containers {
					fmt.Println(format.TruncateID(c.ID))
				}
				return nil
			}

			if cmd.Root().String("format") == "json" {
				return format.JSON(os.Stdout, containers)
			}

			headers := []string{"CONTAINER ID", "IMAGE", "COMMAND", "CREATED", "STATUS", "PORTS", "NAMES"}
			var rows [][]string
			for _, c := range containers {
				rows = append(rows, []string{
					format.TruncateID(c.ID),
					c.Image,
					c.Command,
					format.HumanDuration(c.Created),
					c.Status,
					c.Ports,
					c.Name,
				})
			}
			format.Table(os.Stdout, headers, rows)
			return nil
		},
	}
}
