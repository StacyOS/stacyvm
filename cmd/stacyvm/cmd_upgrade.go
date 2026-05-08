package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type upgradeRehearsalReport struct {
	GeneratedAt       string        `json:"generated_at"`
	ConfigPath        string        `json:"config_path,omitempty"`
	DatabasePath      string        `json:"database_path"`
	BackupOutput      string        `json:"backup_output"`
	ConfigLint        []doctorCheck `json:"config_lint"`
	DatabaseChecks    []doctorCheck `json:"database_checks"`
	Doctor            []doctorCheck `json:"doctor,omitempty"`
	RecommendedSteps  []string      `json:"recommended_steps"`
	ProductionReady   bool          `json:"production_ready"`
	RequiresLiveCheck bool          `json:"requires_live_check"`
}

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Run upgrade preparation checks",
	}
	cmd.AddCommand(newUpgradeRehearseCmd())
	return cmd
}

func newUpgradeRehearseCmd() *cobra.Command {
	var configPath string
	var dbPath string
	var backupOutput string
	var includeDoctor bool
	cmd := &cobra.Command{
		Use:   "rehearse",
		Short: "Rehearse a single-node production upgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := runUpgradeRehearsal(cmd.Context(), upgradeRehearsalOptions{
				ConfigPath:    configPath,
				DatabasePath:  dbPath,
				BackupOutput:  backupOutput,
				IncludeDoctor: includeDoctor,
			})
			if err != nil {
				return err
			}
			return printUpgradeRehearsal(report)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "config file to rehearse; defaults to normal StacyVM config lookup")
	cmd.Flags().StringVar(&dbPath, "database", "", "database path; defaults to configured database.path")
	cmd.Flags().StringVar(&backupOutput, "backup-output", "", "intended backup output path; defaults to a timestamped file next to the database")
	cmd.Flags().BoolVar(&includeDoctor, "include-doctor", false, "also run live host doctor checks")
	return cmd
}

type upgradeRehearsalOptions struct {
	ConfigPath    string
	DatabasePath  string
	BackupOutput  string
	IncludeDoctor bool
}

func runUpgradeRehearsal(ctx context.Context, opts upgradeRehearsalOptions) (*upgradeRehearsalReport, error) {
	cfg, err := loadLintConfig(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	dbPath := opts.DatabasePath
	if dbPath == "" {
		dbPath = cfg.Database.Path
	}
	dbAbs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}

	backupOutput := opts.BackupOutput
	if backupOutput == "" {
		backupOutput = filepath.Join(filepath.Dir(dbAbs), "stacyvm-upgrade-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	}
	backupAbs, err := filepath.Abs(backupOutput)
	if err != nil {
		return nil, err
	}

	report := &upgradeRehearsalReport{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		ConfigPath:        opts.ConfigPath,
		DatabasePath:      dbAbs,
		BackupOutput:      backupAbs,
		ConfigLint:        lintConfig(cfg, true),
		DatabaseChecks:    rehearseDatabaseChecks(ctx, dbAbs, backupAbs),
		RecommendedSteps:  upgradeSteps(dbAbs, backupAbs, opts.ConfigPath),
		RequiresLiveCheck: !opts.IncludeDoctor,
	}
	if opts.IncludeDoctor {
		report.Doctor = runDoctor(ctx, true)
		report.RequiresLiveCheck = false
	}
	report.ProductionReady = allChecksPass(report.ConfigLint) && allChecksPass(report.DatabaseChecks) && (!opts.IncludeDoctor || allChecksPass(report.Doctor))
	return report, nil
}

func rehearseDatabaseChecks(ctx context.Context, dbPath, backupOutput string) []doctorCheck {
	var checks []doctorCheck
	if _, err := os.Stat(dbPath); err != nil {
		checks = append(checks, doctorCheck{Name: "database.source", Status: doctorFail, Message: err.Error(), Remediation: "Run the rehearsal on the host that owns the SQLite database, or pass --database to the active DB path."})
	} else if err := checkSQLiteIntegrity(ctx, dbPath); err != nil {
		checks = append(checks, doctorCheck{Name: "database.integrity", Status: doctorFail, Message: err.Error(), Remediation: "Investigate SQLite integrity before upgrading; do not proceed until integrity_check returns ok."})
	} else {
		checks = append(checks, doctorCheck{Name: "database.integrity", Status: doctorPass, Message: "ok"})
	}

	backupDir := filepath.Dir(backupOutput)
	if info, err := os.Stat(backupDir); err != nil {
		checks = append(checks, doctorCheck{Name: "backup.directory", Status: doctorFail, Message: err.Error(), Remediation: "Create the backup directory and ensure the StacyVM operator can write to it."})
	} else if !info.IsDir() {
		checks = append(checks, doctorCheck{Name: "backup.directory", Status: doctorFail, Message: "path is not a directory", Remediation: "Choose a backup output path whose parent is a directory."})
	} else {
		checks = append(checks, doctorCheck{Name: "backup.directory", Status: doctorPass, Message: backupDir})
	}
	if _, err := os.Stat(backupOutput); err == nil {
		checks = append(checks, doctorCheck{Name: "backup.output", Status: doctorFail, Message: "backup output already exists", Remediation: "Choose a fresh --backup-output path or run stacyvm db backup with --force intentionally."})
	} else if err != nil && !os.IsNotExist(err) {
		checks = append(checks, doctorCheck{Name: "backup.output", Status: doctorFail, Message: err.Error(), Remediation: "Choose a backup output path that can be checked by the operator."})
	} else {
		checks = append(checks, doctorCheck{Name: "backup.output", Status: doctorPass, Message: backupOutput})
	}
	return checks
}

func upgradeSteps(dbPath, backupOutput, configPath string) []string {
	if configPath == "" {
		configPath = "the active StacyVM config"
	}
	return []string{
		fmt.Sprintf("Run stacyvm config lint --production --file %s with the service environment loaded.", configPath),
		fmt.Sprintf("Run stacyvm db backup %s --database %s before replacing binaries or images.", backupOutput, dbPath),
		"Replace the stacyvm binary or update STACYVM_IMAGE.",
		"Restart the service.",
		"Confirm GET /api/v1/ready succeeds before routing traffic.",
		"If readiness fails after upgrade, stop StacyVM and run stacyvm db restore against the pre-upgrade backup.",
	}
}

func printUpgradeRehearsal(report *upgradeRehearsalReport) error {
	fmt.Println("Upgrade rehearsal")
	fmt.Printf("  database: %s\n", report.DatabasePath)
	fmt.Printf("  backup:   %s\n", report.BackupOutput)
	fmt.Println()
	fmt.Println("Config lint:")
	configFailures := printDoctorChecks(report.ConfigLint)
	fmt.Println()
	fmt.Println("Database checks:")
	dbFailures := printDoctorChecks(report.DatabaseChecks)
	doctorFailures := 0
	if len(report.Doctor) > 0 {
		fmt.Println()
		fmt.Println("Doctor checks:")
		doctorFailures = printDoctorChecks(report.Doctor)
	}
	fmt.Println()
	fmt.Println("Recommended upgrade flow:")
	for i, step := range report.RecommendedSteps {
		fmt.Printf("  %d. %s\n", i+1, step)
	}
	if report.RequiresLiveCheck {
		fmt.Println()
		fmt.Println("Live host checks were skipped. Run with --include-doctor or run stacyvm doctor --production on the target host before go-live.")
	}
	if failures := configFailures + dbFailures + doctorFailures; failures > 0 {
		return fmt.Errorf("upgrade rehearsal found %d failing check(s)", failures)
	}
	fmt.Println()
	fmt.Println("Upgrade rehearsal passed.")
	return nil
}

func allChecksPass(checks []doctorCheck) bool {
	for _, check := range checks {
		if check.Status == doctorFail {
			return false
		}
	}
	return true
}
