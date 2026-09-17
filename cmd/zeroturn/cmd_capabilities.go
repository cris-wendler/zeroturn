package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/cris-wendler/zeroturn/internal/capabilities"
	"github.com/cris-wendler/zeroturn/internal/output"
)

const capabilitiesUsage = `zeroturn capabilities [--json]

Print the contract this build speaks: the version, the event types, the
guard modes, the commands, the exit codes, and the fields that are
recorded and never recorded. An adapter should read this rather than
assume.
`

func cmdCapabilities(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	if err := parseFlags(fs, args, capabilitiesUsage, "capabilities"); err != nil {
		return err
	}
	doc := capabilities.Describe(version())
	if *asJSON {
		return output.JSON(os.Stdout, doc)
	}
	fmt.Println("ZEROTURN CAPABILITIES")
	fmt.Print(output.Table([][2]string{
		{"version", doc.Version},
		{"contract version", doc.ContractVersion},
		{"event contract", doc.EventContract},
	}))
	fmt.Println()
	for _, h := range doc.Harnesses {
		fmt.Printf("%s (%s)\n", h.Name, h.Status)
		fmt.Printf("  gate       %v\n", h.GateSupported)
		fmt.Printf("  status     %v\n", h.StatusSupported)
		fmt.Printf("  %s\n", h.Notes)
	}
	fmt.Println("\nRun zeroturn capabilities --json for the full contract.")
	return nil
}
