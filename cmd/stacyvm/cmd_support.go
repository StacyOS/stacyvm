package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/spf13/cobra"
)

type supportBundle struct {
	GeneratedAt       string                 `json:"generated_at"`
	Version           string                 `json:"version"`
	Runtime           map[string]string      `json:"runtime"`
	Config            map[string]interface{} `json:"config,omitempty"`
	ConfigLint        []doctorCheck          `json:"config_lint,omitempty"`
	Doctor            []doctorCheck          `json:"doctor,omitempty"`
	ServerDiagnostics map[string]interface{} `json:"server_diagnostics,omitempty"`
	CollectionErrors  []string               `json:"collection_errors,omitempty"`
	Redactions        []string               `json:"redactions"`
}

func newSupportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "support",
		Short: "Collect redacted support diagnostics",
	}
	cmd.AddCommand(newSupportBundleCmd())
	return cmd
}

func newSupportBundleCmd() *cobra.Command {
	var configPath string
	var includeDoctor bool
	var includeServer bool
	cmd := &cobra.Command{
		Use:   "bundle <output-path>",
		Short: "Write a redacted support bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bundle := collectSupportBundle(cmd.Context(), supportBundleOptions{
				ConfigPath:    configPath,
				IncludeDoctor: includeDoctor,
				IncludeServer: includeServer,
			})
			return writeSupportBundle(args[0], bundle)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "config file to include; defaults to normal StacyVM config lookup")
	cmd.Flags().BoolVar(&includeDoctor, "include-doctor", false, "include local doctor checks")
	cmd.Flags().BoolVar(&includeServer, "include-server", false, "include /api/v1/diagnostics from --server when reachable")
	return cmd
}

type supportBundleOptions struct {
	ConfigPath    string
	IncludeDoctor bool
	IncludeServer bool
}

func collectSupportBundle(ctx context.Context, opts supportBundleOptions) supportBundle {
	bundle := supportBundle{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Version:     version,
		Runtime: map[string]string{
			"goos":   runtime.GOOS,
			"goarch": runtime.GOARCH,
		},
		Redactions: []string{
			"secret-like config keys",
			"API keys",
			"bearer tokens",
			"basic auth credentials in URLs",
		},
	}

	cfg, err := loadLintConfig(opts.ConfigPath)
	if err != nil {
		bundle.CollectionErrors = append(bundle.CollectionErrors, "config: "+err.Error())
	} else {
		bundle.Config = redactConfig(cfg)
		bundle.ConfigLint = redactDoctorChecks(lintConfig(cfg, true))
	}

	if opts.IncludeDoctor {
		bundle.Doctor = redactDoctorChecks(runDoctor(ctx, true))
	}
	if opts.IncludeServer {
		diagnostics, err := fetchServerDiagnostics()
		if err != nil {
			bundle.CollectionErrors = append(bundle.CollectionErrors, "server_diagnostics: "+err.Error())
		} else {
			bundle.ServerDiagnostics = diagnostics
		}
	}
	return bundle
}

func writeSupportBundle(path string, bundle supportBundle) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	data = []byte(redactString(string(data)))
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	fmt.Printf("support bundle written: %s\n", path)
	return nil
}

func fetchServerDiagnostics() (map[string]interface{}, error) {
	client := getClient()
	resp, err := client.do(http.MethodGet, "/api/v1/diagnostics", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var diagnostics map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&diagnostics); err != nil {
		return nil, err
	}
	return redactMap(diagnostics).(map[string]interface{}), nil
}

func redactConfig(cfg *config.Config) map[string]interface{} {
	data, err := json.Marshal(cfg)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return redactMap(raw).(map[string]interface{})
}

func redactDoctorChecks(checks []doctorCheck) []doctorCheck {
	redacted := make([]doctorCheck, len(checks))
	for i, check := range checks {
		redacted[i] = doctorCheck{
			Name:        check.Name,
			Status:      check.Status,
			Message:     redactString(check.Message),
			Remediation: redactString(check.Remediation),
		}
	}
	return redacted
}

func redactMap(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			if isSecretKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactMap(nested)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, nested := range typed {
			out[i] = redactMap(nested)
		}
		return out
	case string:
		return redactString(typed)
	default:
		return typed
	}
}

func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, marker := range []string{"api_key", "apikey", "signing_key", "private_key", "key_file", "token", "secret", "password", "credential", "authorization", "auth_header"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`stacyvm-worker-v1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)(x-api-key|x-admin-api-key|api[_-]?key)\s*[:=]\s*["']?[^"',\s}]+`),
	regexp.MustCompile(`(?i)(password|token|secret|signing[_-]?key|private[_-]?key)\s*[:=]\s*["']?[^"',\s}]+`),
	regexp.MustCompile(`([a-z][a-z0-9+.-]*://)[^:/@\s]+:[^/@\s]+@`),
	regexp.MustCompile(`(?i)sk-[a-z0-9][a-z0-9_-]{8,}`),
}

func redactString(value string) string {
	redacted := value
	for _, pattern := range redactionPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.Contains(match, "://") && strings.Contains(match, "@") {
				parts := pattern.FindStringSubmatch(match)
				if len(parts) > 1 {
					return parts[1] + "[REDACTED]@"
				}
			}
			return "[REDACTED]"
		})
	}
	return redacted
}
