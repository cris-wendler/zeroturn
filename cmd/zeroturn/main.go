// Command zeroturn shows when a coding session is under pressure and runs
// routine development work locally.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cris-wendler/zeroturn/internal/output"
)

// Version is set at build time. The zero value marks a development build.
var Version = "0.1.0-dev"

const usage = `zeroturn - session pressure and local development commands

Usage:
  zeroturn <command> [flags]

Session Guard
  status            Show the condition of the current session
  policy            Show, check, or change guard thresholds
  report            Summarise what ZeroTurn observed locally
  integrate         Plan, apply, or remove a harness integration

Direct Lane
  verify            Run the configured validation steps directly
  ship              Stage named files, commit, and push with checks

Other
  init              Write .zeroturn.json for this repository
  capabilities      Print the machine readable contract
  doctor            Check the local installation and harness support
  version           Print the version
  event             Adapter entry point, reads one event on standard input

Run zeroturn <command> --help for the flags a command accepts.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(output.ExitInvalidUsage)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	defer cancel()

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(ctx, args)
	case "integrate":
		err = cmdIntegrate(ctx, args)
	case "status":
		err = cmdStatus(ctx, args)
	case "policy":
		err = cmdPolicy(ctx, args)
	case "report":
		err = cmdReport(ctx, args)
	case "verify":
		err = cmdVerify(ctx, args)
	case "ship":
		err = cmdShip(ctx, args)
	case "capabilities":
		err = cmdCapabilities(ctx, args)
	case "doctor":
		err = cmdDoctor(ctx, args)
	case "event":
		err = cmdEvent(ctx, args)
	case "version", "--version", "-v":
		fmt.Printf("zeroturn %s\n", Version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "zeroturn has no command named %q\n", cmd)
		fmt.Fprintf(os.Stderr, "  next:   run zeroturn help to list the commands\n")
		os.Exit(output.ExitInvalidUsage)
	}

	if err != nil {
		if ze, ok := err.(*output.Error); ok {
			ze.Print(os.Stderr)
			os.Exit(ze.Code)
		}
		fmt.Fprintf(os.Stderr, "zeroturn stopped\n  reason: %v\n", err)
		os.Exit(output.ExitInternal)
	}
}
