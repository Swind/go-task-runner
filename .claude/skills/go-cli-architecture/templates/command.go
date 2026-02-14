// cmd/COMMAND_NAME.go
package cmd

import (
	"fmt"
	"myapp/internal/service"

	"github.com/urfave/cli/v2"
)

func COMMAND_NAMECommand() *cli.Command {
	return &cli.Command{
		Name:    "COMMAND_NAME",
		Aliases: []string{"SHORT"},
		Usage:   "Description",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Required: true,
				Usage:    "Name of the resource",
			},
		},

		Action: COMMAND_NAMEAction,
	}
}

func COMMAND_NAMEAction(c *cli.Context) error {
	// 1. Get flags
	name := c.String("name")

	// 2. Validate (format only)
	if len(name) < 3 {
		return cli.Exit("name must be at least 3 characters", 1)
	}

	// 3. Call service
	svc := service.NewCOMMAND_NAMEService()
	result, err := svc.COMMAND_NAME(name)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Failed: %v", err), 1)
	}

	// 4. Format output
	fmt.Printf("✓ Success: %v\n", result)

	return nil
}
