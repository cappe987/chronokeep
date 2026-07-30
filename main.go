// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package main

import (
	app "ckeep/app"
	cmd "ckeep/app/cmd"
	"fmt"
	"os"
)

func Usage() {
	fmt.Println("------- ChronoKeep v0.1 -------")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("\tckeep [mode]")
	fmt.Println("")
	fmt.Println("Modes:")
	fmt.Println("\tpkt - Send and receive timestamped packets")
	fmt.Println("\tgm - Run a PTP GM on a port")
	fmt.Println("\tdelay - Run either a client or server for (p)delays")
	fmt.Println("\textts - Listen to EXTTS events")
	fmt.Println("\tdelay - Perform path delay measurements")
	fmt.Println("\tte - Measure time error and accuracy")
	fmt.Println("\tversion - Show version")
}

func main() {
	if len(os.Args) == 1 {
		Usage()
		return
	}

	arg := os.Args[1]
	// Drop the 'mode' arg
	os.Args = os.Args[1:]
	// args := os.Args[2:]
	// fmt.Printf("Mode: %s\n", arg)
	// fmt.Printf("Args: %v\n", os.Args)

	switch arg {
	case "pkt":
		cmd.PktMode()
	case "gm":
		cmd.GmMode()
	case "delay":
		cmd.DelayMode()
	case "te":
		cmd.TeMode()
	case "web":
		app.WebServer()
	case "-v", "--version", "version":
		fmt.Println("ChronoKeep - v0.1")
	default:
		Usage()
	}
}
