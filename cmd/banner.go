package cmd

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/MADPANDA3D/pandaflix/core"
)

// bannerArt is the PANDAFLIX wordmark (ANSI Shadow), shown on interactive runs.
const bannerArt = `██████╗  █████╗ ███╗   ██╗██████╗  █████╗ ███████╗██╗     ██╗██╗  ██╗
██╔══██╗██╔══██╗████╗  ██║██╔══██╗██╔══██╗██╔════╝██║     ██║╚██╗██╔╝
██████╔╝███████║██╔██╗ ██║██║  ██║███████║█████╗  ██║     ██║ ╚███╔╝
██╔═══╝ ██╔══██║██║╚██╗██║██║  ██║██╔══██║██╔══╝  ██║     ██║ ██╔██╗
██║     ██║  ██║██║ ╚████║██████╔╝██║  ██║██║     ███████╗██║██╔╝ ██╗
╚═╝     ╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝ ╚═╝  ╚═╝╚═╝     ╚══════╝╚═╝╚═╝  ╚═╝`

// printBanner renders the wordmark for interactive terminals only, so piped
// output and scripts stay clean.
func printBanner() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	fmt.Printf("\x1b[91m%s\x1b[0m\n", bannerArt)
	if core.Version != "" {
		fmt.Printf("\x1b[2m  maintained fork · %s\x1b[0m\n\n", core.Version)
	} else {
		fmt.Println()
	}
}
