package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"

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

			// The dashboard makes same-origin API calls to /api/v1 (see
			// web/api/client.ts: `const BASE = process.env.NEXT_PUBLIC_API_URL || '/api/v1'`).
			// The embedded build has no baked-in API URL, so it uses that relative
			// path. Reverse-proxy /api and /swagger to the running StacyVM server so
			// those calls resolve without CORS or a hard-coded API host.
			apiTarget, err := url.Parse(serverURL)
			if err != nil {
				return fmt.Errorf("invalid --server URL %q: %w", serverURL, err)
			}
			proxy := httputil.NewSingleHostReverseProxy(apiTarget)

			mux := http.NewServeMux()
			mux.Handle("/api/", proxy)
			mux.Handle("/swagger/", proxy)
			mux.Handle("/", http.FileServer(http.FS(subFS)))

			addr := fmt.Sprintf("localhost:%d", webPort)
			pageURL := fmt.Sprintf("http://%s", addr)

			fmt.Printf("Starting web dashboard at %s\n", pageURL)
			fmt.Printf("Proxying API requests to %s\n", serverURL)
			fmt.Println("Press Ctrl+C to exit")

			// Open the browser automatically
			if err := browser.OpenURL(pageURL); err != nil {
				fmt.Printf("Failed to open browser automatically: %v\n", err)
				fmt.Printf("Please open %s in your web browser.\n", pageURL)
			}

			// Start the server
			if err := http.ListenAndServe(addr, mux); err != nil {
				return fmt.Errorf("web server failed: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&webPort, "port", "p", 5749, "Port to serve the web UI on")

	return cmd
}
