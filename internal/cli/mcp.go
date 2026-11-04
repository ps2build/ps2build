package cli

import (
	"github.com/spf13/cobra"

	"github.com/ps2build/ps2build/internal/mcpserver"
)

var mcpListen string

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP server exposing installed packages and ps2.yaml tooling",
	Long: "Exposes package lookup, docs, libs: resolution, validation, and building/installing as MCP tools.\n\n" +
		"Serves over stdio by default; --listen serves HTTP instead (unauthenticated - only bind a trusted address).",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mcpListen != "" {
			return mcpserver.ServeHTTP(cmd.Context(), mcpListen)
		}
		return mcpserver.Serve(cmd.Context())
	},
}

func init() {
	mcpserver.SetVersion(version)
	mcpCmd.Flags().StringVar(&mcpListen, "listen", "", "serve streamable HTTP on this address (e.g. localhost:8931) instead of stdio")
	rootCmd.AddCommand(mcpCmd)
}
