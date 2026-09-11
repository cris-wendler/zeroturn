package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/cris-wendler/zeroturn/internal/capabilities"
	"github.com/cris-wendler/zeroturn/internal/output"
)

func cmdCapabilities(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print machine readable output")
	if err := fs.Parse(args); err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn could not read the flags", err.Error(),
			"run zeroturn capabilities --json")
	}
	doc := capabilities.Describe(Version)
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
