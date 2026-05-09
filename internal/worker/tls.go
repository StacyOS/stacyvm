package worker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type TLSConfig struct {
	Enabled            bool
	ServerCertFile     string
	ServerKeyFile      string
	ClientCAFile       string
	CAFile             string
	ClientCertFile     string
	ClientKeyFile      string
	ServerName         string
	InsecureSkipVerify bool
}

func (c TLSConfig) ServerConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}
	if strings.TrimSpace(c.ServerCertFile) == "" || strings.TrimSpace(c.ServerKeyFile) == "" {
		return nil, fmt.Errorf("worker RPC TLS server cert and key files are required")
	}
	cert, err := tls.LoadX509KeyPair(c.ServerCertFile, c.ServerKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load worker RPC server certificate: %w", err)
	}
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}
	if strings.TrimSpace(c.ClientCAFile) != "" {
		pool, err := loadCertPool(c.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("load worker RPC client CA: %w", err)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}

func (c TLSConfig) HTTPClient(timeoutSource *http.Client) (*http.Client, error) {
	if !c.Enabled {
		return timeoutSource, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         strings.TrimSpace(c.ServerName),
		InsecureSkipVerify: c.InsecureSkipVerify,
	}
	if strings.TrimSpace(c.CAFile) != "" {
		pool, err := loadCertPool(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("load worker RPC CA: %w", err)
		}
		tlsConfig.RootCAs = pool
	}
	if strings.TrimSpace(c.ClientCertFile) != "" || strings.TrimSpace(c.ClientKeyFile) != "" {
		if strings.TrimSpace(c.ClientCertFile) == "" || strings.TrimSpace(c.ClientKeyFile) == "" {
			return nil, fmt.Errorf("worker RPC TLS client cert and key files must be configured together")
		}
		cert, err := tls.LoadX509KeyPair(c.ClientCertFile, c.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load worker RPC client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	transport.TLSClientConfig = tlsConfig
	client := &http.Client{Transport: transport}
	if timeoutSource != nil {
		client.Timeout = timeoutSource.Timeout
		client.CheckRedirect = timeoutSource.CheckRedirect
		client.Jar = timeoutSource.Jar
	}
	return client, nil
}

func loadCertPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no PEM certificates found in %s", path)
	}
	return pool, nil
}
