package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// sshFlags are shared by the ssh subcommands.
type sshFlags struct {
	useWS       bool
	gatewayHost string
	port        int
	ttl         time.Duration
}

func newSSHCmd() *cobra.Command {
	f := &sshFlags{}
	cmd := &cobra.Command{
		Use:   "ssh <sandbox-id>",
		Short: "Open an interactive shell in a sandbox over SSH",
		Long: "Connect to a sandbox's shell over SSH. By default this mints a short-lived\n" +
			"certificate via the API and tunnels SSH over the authenticated WebSocket, so\n" +
			"no extra port needs to be exposed. Use subcommands to write an ssh-config\n" +
			"entry (so plain ssh/scp/VS Code work) or to act as an ssh ProxyCommand.",
		Example: "  stacyvm ssh sb-a1b2c3d4\n" +
			"  stacyvm ssh config sb-a1b2c3d4   # write ~/.ssh/config, then: ssh stacy-sb-a1b2c3d4\n" +
			"  stacyvm ssh sb-a1b2c3d4 --ws=false --gateway-host gw.example.com --port 2222",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSSHConnect(cmd.Context(), f, args[0])
		},
	}
	cmd.PersistentFlags().BoolVar(&f.useWS, "ws", true, "tunnel SSH over the authenticated WebSocket (no extra port needed)")
	cmd.PersistentFlags().StringVar(&f.gatewayHost, "gateway-host", "", "SSH gateway host for native connections (default: server host)")
	cmd.PersistentFlags().IntVar(&f.port, "port", 2222, "native SSH gateway port (when --ws=false)")
	cmd.PersistentFlags().DurationVar(&f.ttl, "ttl", 10*time.Minute, "certificate lifetime")

	cmd.AddCommand(newSSHProxyCmd(f), newSSHConfigCmd(f))
	return cmd
}

func newSSHProxyCmd(f *sshFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "proxy <sandbox-id>",
		Short: "Relay an SSH connection over the WebSocket tunnel (for ssh ProxyCommand)",
		Long: "Used as an ssh ProxyCommand: relays the local ssh client's transport over\n" +
			"the authenticated WebSocket tunnel. Not normally run by hand.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := dialWSTunnel(cmd.Context(), serverURL, apiKey)
			if err != nil {
				return fmt.Errorf("connect tunnel: %w", err)
			}
			return proxyRelay(conn, os.Stdin, os.Stdout)
		},
	}
}

func newSSHConfigCmd(f *sshFlags) *cobra.Command {
	var (
		alias      string
		configPath string
		identity   string
		direct     bool
		printOnly  bool
	)
	cmd := &cobra.Command{
		Use:   "config <sandbox-id>",
		Short: "Write an ~/.ssh/config block so plain ssh/scp/VS Code can reach a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sandbox := args[0]
			if alias == "" {
				alias = "stacy-" + sandbox
			}
			block, err := buildConfigBlockForSandbox(f, sandbox, alias, identity, direct)
			if err != nil {
				return err
			}
			if printOnly {
				fmt.Fprintln(cmd.OutOrStdout(), block)
				return nil
			}
			if configPath == "" {
				configPath, err = defaultSSHConfigPath()
				if err != nil {
					return err
				}
			}
			if err := writeManagedSSHConfig(configPath, alias, block); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s — connect with: ssh %s\n", configPath, alias)
			return nil
		},
	}
	cmd.Flags().StringVar(&alias, "alias", "", "Host alias to write (default: stacy-<sandbox>)")
	cmd.Flags().StringVar(&configPath, "config-path", "", "ssh config file to update (default: ~/.ssh/config)")
	cmd.Flags().StringVar(&identity, "identity", "", "IdentityFile (your registered SSH private key) to use")
	cmd.Flags().BoolVar(&direct, "direct", false, "use the native SSH port directly instead of the WebSocket ProxyCommand")
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the config block to stdout instead of writing the file")
	return cmd
}

// buildConfigBlockForSandbox renders the ssh-config block for either the
// WebSocket-ProxyCommand flow (default) or a direct native-port connection.
func buildConfigBlockForSandbox(f *sshFlags, sandbox, alias, identity string, direct bool) (string, error) {
	b := sshConfigBlock{Alias: alias, Sandbox: sandbox, Identity: identity}
	if direct {
		host := f.gatewayHost
		if host == "" {
			h, err := hostFromServerURL(serverURL)
			if err != nil {
				return "", err
			}
			host = h
		}
		b.HostName = host
		b.Port = f.port
		return renderSSHConfigBlock(b), nil
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "stacyvm"
	}
	b.ProxyCmd = fmt.Sprintf("%s ssh proxy %s --server %s", exe, sandbox, serverURL)
	return renderSSHConfigBlock(b), nil
}

func hostFromServerURL(serverURL string) (string, error) {
	u, err := wsURLFromServer(serverURL, "")
	if err != nil {
		return "", err
	}
	// reuse parsing: strip scheme via net/url through wsURLFromServer is overkill;
	// just split host:port off the ws URL.
	trimmed := strings.TrimPrefix(strings.TrimPrefix(u, "wss://"), "ws://")
	host := trimmed
	if i := strings.IndexAny(trimmed, ":/"); i >= 0 {
		host = trimmed[:i]
	}
	if host == "" {
		return "", fmt.Errorf("could not derive host from server URL %q", serverURL)
	}
	return host, nil
}

func defaultSSHConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// writeManagedSSHConfig idempotently inserts/updates the managed block for alias
// in the ssh config file, creating ~/.ssh with safe permissions if needed.
func writeManagedSSHConfig(path, alias, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated := upsertSSHConfigBlock(string(existing), alias, block)
	return os.WriteFile(path, []byte(updated), 0o600)
}

// runSSHConnect mints a certificate, establishes the SSH transport (WebSocket or
// native), and runs an interactive shell with live terminal resizing.
func runSSHConnect(ctx context.Context, f *sshFlags, sandbox string) error {
	signer, authLine, err := generateEphemeralSigner()
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	certLine, err := requestSSHCert(getClient(), sandbox, authLine, f.ttl)
	if err != nil {
		return fmt.Errorf("request certificate: %w", err)
	}
	certSigner, err := buildCertSigner(signer, certLine)
	if err != nil {
		return err
	}

	transport, addr, err := dialSSHTransport(ctx, f)
	if err != nil {
		return err
	}

	cfg := &gossh.ClientConfig{
		User: sandbox,
		Auth: []gossh.AuthMethod{gossh.PublicKeys(certSigner)},
		// The WebSocket transport is already TLS-authenticated to the control
		// plane; native host-key pinning (known_hosts) is a follow-up.
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	sshConn, chans, reqs, err := gossh.NewClientConn(transport, addr, cfg)
	if err != nil {
		_ = transport.Close()
		return fmt.Errorf("ssh handshake: %w", err)
	}
	client := gossh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer sess.Close()

	fd := int(os.Stdin.Fd())
	cols, rows := 80, 24
	if term.IsTerminal(fd) {
		if w, h, err := term.GetSize(fd); err == nil {
			cols, rows = w, h
		}
	}
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color"
	}
	if err := sess.RequestPty(termEnv, rows, cols, gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	sess.Stdin = os.Stdin
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr

	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err == nil {
			defer func() { _ = term.Restore(fd, state) }()
		}
	}

	if err := sess.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}
	stop := watchWindowResize(sess, fd)
	defer stop()

	err = sess.Wait()
	var exitErr *gossh.ExitError
	if err != nil && asExitError(err, &exitErr) {
		os.Exit(exitErr.ExitStatus())
	}
	return err
}

func asExitError(err error, target **gossh.ExitError) bool {
	if e, ok := err.(*gossh.ExitError); ok {
		*target = e
		return true
	}
	return false
}

// dialSSHTransport returns a raw SSH transport (WebSocket-backed or native TCP)
// and the address string to use for the SSH handshake.
func dialSSHTransport(ctx context.Context, f *sshFlags) (net.Conn, string, error) {
	if f.useWS {
		conn, err := dialWSTunnel(ctx, serverURL, apiKey)
		if err != nil {
			return nil, "", fmt.Errorf("connect tunnel: %w", err)
		}
		return conn, "stacyvm-ws", nil
	}
	host := f.gatewayHost
	if host == "" {
		h, err := hostFromServerURL(serverURL)
		if err != nil {
			return nil, "", err
		}
		host = h
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", f.port))
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, "", fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, addr, nil
}
