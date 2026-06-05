package providers

import (
	"context"
	"fmt"
	"net"
	"time"
)

// DialSandbox dials addr ("host:port") for SSH port forwarding. A loopback
// target (e.g. localhost:3000 from `ssh -L`) refers to a service inside the
// container, so it is rewritten to the container's bridge IP — this reaches
// services bound to 0.0.0.0 inside the container. Non-loopback hosts are dialed
// as given. It satisfies providers.DialProvider.
func (d *DockerProvider) DialSandbox(ctx context.Context, sandboxID, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid forward address %q: %w", addr, err)
	}
	if isLoopbackHost(host) {
		ip, err := d.containerIP(ctx, sandboxID)
		if err != nil {
			return nil, err
		}
		host = ip
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// containerIP returns the container's primary bridge IP address.
func (d *DockerProvider) containerIP(ctx context.Context, sandboxID string) (string, error) {
	info, err := d.cli.ContainerInspect(ctx, sandboxID)
	if err != nil {
		return "", fmt.Errorf("inspect container %s: %w", sandboxID, err)
	}
	if info.NetworkSettings != nil {
		if ip := info.NetworkSettings.IPAddress; ip != "" {
			return ip, nil
		}
		for _, n := range info.NetworkSettings.Networks {
			if n != nil && n.IPAddress != "" {
				return n.IPAddress, nil
			}
		}
	}
	return "", fmt.Errorf("container %s has no reachable IP address", sandboxID)
}

// isLoopbackHost reports whether host names the local loopback (or 0.0.0.0),
// indicating a service inside the sandbox rather than an external address.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "0.0.0.0", "":
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return false
}
