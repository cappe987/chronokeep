package main

import (
	"fmt"
	"os"
	"ptpan/cmd"

	"github.com/pborman/getopt/v2"
)

func main() {
	mode := os.Args[1]
	args := os.Args[2:]
	os.Args = append(os.Args[:1], args...)

	fmt.Printf("Mode: %s\n", mode)
	if mode == "pkt" {
		cmd.PktMode(args)
		return
	}

	version := false
	s := ""
	help := false

	getopt.FlagLong(&version, "version", 'v', "Print version")
	getopt.FlagLong(&s, "iface", 'i', "Interface to use")
	getopt.FlagLong(&help, "help", 'h', "Print this menu")

	getopt.Parse()

	if help {
		getopt.Usage()
		return
	}
	if version {
		// All-in-one PTP tool
		fmt.Println("aiop - v0.1")
		return
	}

	fmt.Println(s, version, help)
}
