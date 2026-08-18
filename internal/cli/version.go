package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xavimf87/kubewhy/internal/version"
)

func newVersionCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Get()
			if opts.output == "json" {
				encoder := json.NewEncoder(opts.stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(info)
			}
			fmt.Fprintln(opts.stdout, info)
			return nil
		},
	}
}
