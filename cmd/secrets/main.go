package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"image-agent/internal/app"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: secrets <import-env|rotate-key>")
		return 2
	}
	databaseURL, masterKey, keyVersion, err := app.LoadSecretsBootstrapFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	db, err := app.OpenDatabase(databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database connection failed")
		return 1
	}
	store, err := app.NewSecretStore(db, masterKey, keyVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "secret table migration failed")
		return 1
	}

	switch args[0] {
	case "import-env":
		flags := flag.NewFlagSet("import-env", flag.ContinueOnError)
		actor := flags.String("actor", "cli:import-env", "audit actor")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		count, err := app.ImportRuntimeSecretsFromEnv(ctx, store, strings.TrimSpace(*actor))
		if err != nil {
			fmt.Fprintln(os.Stderr, "secret import failed")
			return 1
		}
		fmt.Printf("imported %d encrypted runtime secrets\n", count)
		return 0
	case "rotate-key":
		flags := flag.NewFlagSet("rotate-key", flag.ContinueOnError)
		actor := flags.String("actor", "cli:rotate-key", "audit actor")
		if err := flags.Parse(args[1:]); err != nil {
			return 2
		}
		encoded, err := app.ReadSecretValueFromEnv("APP_SECRETS_NEW_MASTER_KEY")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if strings.TrimSpace(encoded) == "" {
			fmt.Fprintln(os.Stderr, "APP_SECRETS_NEW_MASTER_KEY(_FILE) is required")
			return 2
		}
		newKey, err := app.DecodeSecretsMasterKey(encoded)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if err := store.RotateMasterKey(ctx, newKey, strings.TrimSpace(*actor)); err != nil {
			if errors.Is(err, app.ErrSecretNotFound) {
				fmt.Fprintln(os.Stderr, "no secrets found")
			} else {
				fmt.Fprintln(os.Stderr, "secret rotation failed")
			}
			return 1
		}
		fmt.Printf("rotated encrypted secrets to key version %d\n", keyVersion+1)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: secrets <import-env|rotate-key>")
		return 2
	}
}
