//go:build !darwin

package cli

import "github.com/spf13/cobra"

func addSessionStop(root *cobra.Command, e *env) {}
