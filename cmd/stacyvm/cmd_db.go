package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Manage the StacyVM SQLite database",
	}
	cmd.AddCommand(newDBBackupCmd(), newDBRestoreCmd())
	return cmd
}

func newDBBackupCmd() *cobra.Command {
	var dbPath string
	var force bool
	cmd := &cobra.Command{
		Use:   "backup <output-path>",
		Short: "Create a consistent SQLite backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := resolveDatabasePath(dbPath)
			if err != nil {
				return err
			}
			return backupSQLite(cmd.Context(), source, args[0], force)
		},
	}
	cmd.Flags().StringVar(&dbPath, "database", "", "database path; defaults to configured database.path")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing backup output file")
	return cmd
}

func newDBRestoreCmd() *cobra.Command {
	var dbPath string
	var yes bool
	cmd := &cobra.Command{
		Use:   "restore <backup-path>",
		Short: "Restore SQLite database from a backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("restore replaces the active database; rerun with --yes after stopping stacyvm")
			}
			target, err := resolveDatabasePath(dbPath)
			if err != nil {
				return err
			}
			return restoreSQLite(cmd.Context(), args[0], target)
		},
	}
	cmd.Flags().StringVar(&dbPath, "database", "", "database path; defaults to configured database.path")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the service is stopped and restore should proceed")
	return cmd
}

func resolveDatabasePath(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return filepath.Abs(cfg.Database.Path)
}

func backupSQLite(ctx context.Context, source, output string, force bool) error {
	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if _, err := os.Stat(sourceAbs); err != nil {
		return fmt.Errorf("source database unavailable: %w", err)
	}
	if _, err := os.Stat(outputAbs); err == nil && !force {
		return fmt.Errorf("backup output already exists: %s; use --force to overwrite", outputAbs)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputAbs), 0750); err != nil {
		return err
	}
	if force {
		if err := os.Remove(outputAbs); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	db, err := sql.Open("sqlite", sourceAbs+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", outputAbs); err != nil {
		return fmt.Errorf("creating sqlite backup: %w", err)
	}
	if err := checkSQLiteIntegrity(ctx, outputAbs); err != nil {
		return fmt.Errorf("backup integrity check failed: %w", err)
	}
	fmt.Printf("backup written: %s\n", outputAbs)
	return nil
}

func restoreSQLite(ctx context.Context, backup, target string) error {
	backupAbs, err := filepath.Abs(backup)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := checkSQLiteIntegrity(ctx, backupAbs); err != nil {
		return fmt.Errorf("backup integrity check failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetAbs), 0750); err != nil {
		return err
	}
	if _, err := os.Stat(targetAbs); err == nil {
		safety := targetAbs + ".pre-restore-" + time.Now().UTC().Format("20060102T150405Z")
		if err := copyFile(targetAbs, safety, 0600); err != nil {
			return fmt.Errorf("creating pre-restore safety copy: %w", err)
		}
		fmt.Printf("existing database safety copy: %s\n", safety)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(targetAbs + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing stale sqlite sidecar %s: %w", targetAbs+suffix, err)
		}
	}
	if err := copyFile(backupAbs, targetAbs, 0600); err != nil {
		return fmt.Errorf("restoring database: %w", err)
	}
	fmt.Printf("database restored: %s\n", targetAbs)
	return nil
}

func checkSQLiteIntegrity(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer db.Close()

	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned %q", result)
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
