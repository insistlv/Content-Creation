package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"image-agent/internal/app"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: admin create --username <name>")
		return 2
	}
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	username := flags.String("username", "", "administrator username")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if strings.TrimSpace(*username) == "" {
		fmt.Fprintln(os.Stderr, "--username is required")
		return 2
	}
	databaseURL, _, _, err := app.LoadSecretsBootstrapFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	password, err := readPassword("Password: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read password failed")
		return 1
	}
	confirmation, err := readPassword("Confirm password: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read password failed")
		return 1
	}
	if password != confirmation {
		fmt.Fprintln(os.Stderr, "passwords do not match")
		return 2
	}
	db, err := app.OpenDatabase(databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database connection failed")
		return 1
	}
	if err := app.CreateInitialAdmin(db, *username, password); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("initial administrator created")
	return 0
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(value), err
	}
	value, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(value, "\r\n"), err
}
