package main

import (
	"fmt"
	"strings"

	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	var shell bool
	cmd := &cobra.Command{
		Use:   "exec <sandbox-id> -- <command>",
		Short: "Execute a command in a sandbox",
		Example: `  stacyvm exec sb-a1b2c3d4 -- echo hello
  stacyvm exec sb-a1b2c3d4 -- ls -la
  stacyvm exec sb-a1b2c3d4 --shell -- "echo $HOME && pwd"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sandboxID := args[0]

			// Find everything after "--".
			var commandArgs []string
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx >= 0 && dashIdx < len(args) {
				commandArgs = args[dashIdx:]
			} else if len(args) > 1 {
				commandArgs = args[1:]
			} else {
				return fmt.Errorf("no command specified; use: stacyvm exec <id> -- <command>")
			}
			if len(commandArgs) == 0 {
				return fmt.Errorf("no command specified; use: stacyvm exec <id> -- <command>")
			}

			req := orchestrator.ExecRequest{Mode: "argv", Command: commandArgs[0]}
			if len(commandArgs) > 1 {
				req.Args = commandArgs[1:]
			}
			if shell {
				req.Mode = "shell"
				req.Command = strings.Join(commandArgs, " ")
				req.Args = nil
			}

			c := getClient()
			resp, err := c.do("POST", "/api/v1/sandboxes/"+sandboxID+"/exec", req)
			if err != nil {
				return err
			}

			var result orchestrator.ExecResult
			if err := c.decodeJSON(resp, &result); err != nil {
				return err
			}

			if result.Stdout != "" {
				fmt.Print(result.Stdout)
			}
			if result.Stderr != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s", result.Stderr)
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("exit code: %d", result.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&shell, "shell", false, "run the command through /bin/sh -c instead of argv mode")
	return cmd
}
