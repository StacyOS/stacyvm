package main

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/StacyOs/stacyvm/web"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var webPort int

func newWebUICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "web-ui",
		Aliases: []string{"ui"},
		Short:   "Launch web-based dashboard directly from CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Extract the "out" directory from the embedded FS
			subFS, err := fs.Sub(web.Assets, "out")
			if err != nil {
				return fmt.Errorf("failed to load web assets: %w", err)
			}

			// Create a file server for the assets
			fileServer := http.FileServer(http.FS(subFS))

			http.Handle("/", fileServer)

			addr := fmt.Sprintf("localhost:%d", webPort)
			url := fmt.Sprintf("http://%s", addr)

			fmt.Printf("Starting web dashboard at %s\n", url)
			fmt.Println("Press Ctrl+C to exit")

			// Open the browser automatically
			if err := browser.OpenURL(url); err != nil {
				fmt.Printf("Failed to open browser automatically: %v\n", err)
				fmt.Printf("Please open %s in your web browser.\n", url)
			}

			// Start the server
			if err := http.ListenAndServe(addr, nil); err != nil {
				return fmt.Errorf("web server failed: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&webPort, "port", "p", 5749, "Port to serve the web UI on")

	return cmd
}
