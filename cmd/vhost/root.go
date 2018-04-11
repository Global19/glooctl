package vhost

import (
	"github.com/solo-io/glooctl/pkg/client"
	"github.com/spf13/cobra"
)

var (
	output string
	tplt   string
)

func VHostCmd(opts *client.StorageOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "virtualhost",
		Short: "manage virtual hosts",
	}
	pflags := cmd.PersistentFlags()
	pflags.StringVarP(&output, "output", "o", "", "output format yaml|json|template")
	pflags.StringVarP(&tplt, "template", "t", "", "output template")
	cmd.AddCommand(createCmd(opts), deleteCmd(opts), getCmd(opts),
		updateCmd(opts), editCmd(opts))
	return cmd
}
