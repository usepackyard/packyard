package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/usepackyard/packyard/internal/cli"
)

// runWizard drives the huh forms. The plan is mutated in place; on
// success every required field is populated and ready for doInstall.
// On user cancel (Ctrl-C) huh returns an error and we propagate it.
func runWizard(p *installPlan, f *initFlags) error {
	// Group 1 — basics
	portStr := strconv.Itoa(p.Port)

	basics := huh.NewGroup(
		huh.NewInput().
			Title("HTTP listen port").
			Description("Packyard binds this port on the local host.").
			Value(&portStr).
			Validate(func(s string) error {
				n, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil || n < 1 || n > 65535 {
					return fmt.Errorf("port must be an integer 1-65535")
				}
				if f.forcePort {
					return nil
				}
				free, err := cli.PortAvailable(n)
				if err != nil {
					return err
				}
				if !free {
					return fmt.Errorf("port %d is in use; pick another or restart the install with --force-port", n)
				}
				return nil
			}),

		huh.NewInput().
			Title("Public URL").
			Description("Composer clients will hit this URL. Point it at the reverse proxy if you have one; otherwise http://<host>:<port>.").
			Value(&p.BaseURL).
			Validate(func(s string) error {
				u, err := url.Parse(strings.TrimSpace(s))
				if err != nil {
					return fmt.Errorf("invalid URL: %v", err)
				}
				if u.Scheme != "http" && u.Scheme != "https" {
					return fmt.Errorf("URL must be http:// or https://")
				}
				if u.Host == "" {
					return fmt.Errorf("URL must include a host")
				}
				return nil
			}),
	)

	// Group 2 — database
	dbOpts := []huh.Option[string]{
		huh.NewOption("SQLite (recommended — zero config, single file)", "sqlite"),
		huh.NewOption("MySQL / MariaDB", "mysql"),
		huh.NewOption("PostgreSQL", "postgres"),
	}
	dbDriverField := huh.NewSelect[string]().
		Title("Database").
		Options(dbOpts...).
		Value(&p.DBDriver)
	database := huh.NewGroup(dbDriverField)

	// Group 2b — mysql/postgres credentials (only when needed)
	dbPortStr := strconv.Itoa(p.DBPort)
	dbCreds := huh.NewGroup(
		huh.NewInput().Title("Database host").Value(&p.DBHost),
		huh.NewInput().Title("Database port").Value(&dbPortStr).Validate(func(s string) error {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("port must be 1-65535")
			}
			return nil
		}),
		huh.NewInput().Title("Database name").Value(&p.DBName),
		huh.NewInput().Title("Database user").Value(&p.DBUser),
		huh.NewInput().Title("Database password").Value(&p.DBPassword).EchoMode(huh.EchoModePassword),
		huh.NewInput().Title("SSL mode").Description("disable | require | verify-ca | verify-full (driver-dependent)").Value(&p.DBSSLMode),
	).WithHideFunc(func() bool {
		return p.DBDriver == "sqlite"
	})

	// Group 3 — storage
	storageOpts := []huh.Option[string]{
		huh.NewOption("Local filesystem (recommended for most installs)", "local"),
		huh.NewOption("S3 / S3-compatible (R2, MinIO)", "s3"),
	}
	storage := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Storage").
			Options(storageOpts...).
			Value(&p.StorageType),
		huh.NewInput().
			Title("Storage directory").
			Description("Where package zips are kept.").
			Value(&p.StorageLocal).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("storage directory is required")
				}
				return nil
			}),
	).WithHideFunc(func() bool {
		return p.StorageType != "local"
	})

	s3 := huh.NewGroup(
		huh.NewInput().Title("S3 bucket").Value(&p.S3Bucket),
		huh.NewInput().Title("S3 region").Value(&p.S3Region),
		huh.NewInput().Title("S3 endpoint").Description("Leave blank for AWS; set for R2/MinIO/etc.").Value(&p.S3Endpoint),
		huh.NewInput().Title("S3 access key").Value(&p.S3AccessKey),
		huh.NewInput().Title("S3 secret key").EchoMode(huh.EchoModePassword).Value(&p.S3SecretKey),
	).WithHideFunc(func() bool {
		return p.StorageType != "s3"
	})

	// Group 4 — admin user
	pwMode := "generate"
	if p.AdminPassword != "" {
		pwMode = "keep"
	}
	typed := p.AdminPassword
	confirm := ""
	admin := huh.NewGroup(
		huh.NewInput().Title("Admin email").Value(&p.AdminEmail).Validate(func(s string) error {
			if !strings.Contains(s, "@") {
				return fmt.Errorf("not a valid email")
			}
			return nil
		}),
		huh.NewSelect[string]().
			Title("Admin password").
			Options(
				huh.NewOption("Auto-generate a strong password", "generate"),
				huh.NewOption("Type one myself", "type"),
			).
			Value(&pwMode),
	)

	// Password entry is gated behind its own group so we can hide the
	// whole pair when "generate" is selected. huh hides groups, not
	// individual fields.
	adminPassword := huh.NewGroup(
		huh.NewInput().Title("Type password").EchoMode(huh.EchoModePassword).Value(&typed).Validate(func(s string) error {
			if len(s) < 8 {
				return fmt.Errorf("must be at least 8 characters")
			}
			return nil
		}),
		huh.NewInput().Title("Type password again").EchoMode(huh.EchoModePassword).Value(&confirm).Validate(func(s string) error {
			if s != typed {
				return fmt.Errorf("does not match")
			}
			return nil
		}),
	).WithHideFunc(func() bool { return pwMode != "type" })

	// Group 5 — service install (only if systemd/launchd is present)
	service := huh.NewGroup(
		huh.NewConfirm().
			Title("Install as a system service?").
			Description("Uses systemd on Linux, launchd on macOS. Service starts on boot and restarts on crash.").
			Value(&p.InstallService),
	).WithHideFunc(func() bool {
		return cli.DetectServiceManager() == cli.ServiceNone
	})

	form := huh.NewForm(basics, database, dbCreds, storage, s3, admin, adminPassword, service).
		WithTheme(huh.ThemeBase16()).
		WithShowHelp(true)

	if err := form.Run(); err != nil {
		return err
	}

	// Carry the text-typed values from the wizard back into the plan.
	if n, err := strconv.Atoi(strings.TrimSpace(portStr)); err == nil {
		p.Port = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(dbPortStr)); err == nil {
		p.DBPort = n
	}

	// Resolve the admin password based on the selection.
	switch pwMode {
	case "type":
		p.AdminPassword = typed
		p.AdminGenerated = false
	case "keep":
		// user-supplied via flag/env before wizard — unchanged
	default:
		// "generate" path: leave AdminPassword empty so resolvePlan
		// generates one later (it needs to run after the wizard so
		// the generated value is shown only in the final report).
		p.AdminPassword = ""
	}
	return nil
}
