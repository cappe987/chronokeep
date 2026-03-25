package main

import (
	"fmt"
	"os"
	"ptpan/cmd"
	"time"

	"github.com/pborman/getopt/v2"
)

type Config struct {
	Age        int
	Cats       []string
	Pi         float64
	Perfection []int
	DOB        time.Time
}

func main() {
	mode := os.Args[1]
	args := os.Args[2:]
	// os.Args = append(os.Args[:1], args...)

	// var conf Config
	// _, err := toml.DecodeFile("example.toml", &conf)
	// fmt.Printf("%v\n", err)
	// fmt.Printf("Age %d\n", conf.Age)

	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Args: %v\n", os.Args)
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
		fmt.Println("intime - v0.1")
		return
	}

	fmt.Println(s, version, help)
}
