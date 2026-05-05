package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// version is replaced by release builds.
var version = "dev"

type versionDocument struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

func addVersionCommand(root *cobra.Command, rootOptions *rootFlags) {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print Moth version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return newInvalidArgumentsError(errors.New("version accepts no positional arguments"))
			}
			return renderResult(cmd, rootOptions.Output, versionDocument{
				Type:    "version",
				Version: version,
			})
		},
	}
	root.AddCommand(cmd)
}
