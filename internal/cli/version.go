package cli

import "github.com/spf13/cobra"

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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderResult(cmd, rootOptions.Output, versionDocument{
				Type:    "version",
				Version: version,
			})
		},
	}
	root.AddCommand(cmd)
}
