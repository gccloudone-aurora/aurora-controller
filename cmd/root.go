package cmd

import (
	"github.com/spf13/cobra"
)

var apiserver string
var kubeconfig string

var rootCmd = &cobra.Command{
	Use:   "aurora-controller",
	Short: "A set of controllers that help to further configure the Aurora platform.",
	Long:  `A set of controllers that help to further configure the Aurora platform.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiserver, "apiserver", "", "URL to the Kubernetes API server")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to the Kubeconfig file")
}

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}
