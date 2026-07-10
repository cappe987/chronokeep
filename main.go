package main

import (
	"fmt"
	app "intime/app"
	cmd "intime/app/cmd"
	"os"
)

func Usage() {
	fmt.Println("------- InTime v0.1 -------")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("\tintime [mode]")
	fmt.Println("")
	fmt.Println("Modes:")
	fmt.Println("\tpkt - Send and receive timestamped packets")
	fmt.Println("\tgm - Run a PTP GM on a port")
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
	if arg == "pkt" {
		cmd.PktMode()
	} else if arg == "gm" {
		cmd.GmMode()
	} else if arg == "te" {
		cmd.TeMode()
	} else if arg == "web" {
		app.WebServer()
	} else if arg == "-v" || arg == "--version" || arg == "version" {
		fmt.Println("intime - v0.1")
	} else {
		Usage()
	}
}
