package main

import (
	"fmt"
	"intime/cmd"
	"os"
	"time"
)

type Config struct {
	Age        int
	Cats       []string
	Pi         float64
	Perfection []int
	DOB        time.Time
}

func Usage() {
	fmt.Println("--- InTime v0.1 ---")
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

	// var conf Config
	// _, err := toml.DecodeFile("example.toml", &conf)
	// fmt.Printf("%v\n", err)
	// fmt.Printf("Age %d\n", conf.Age)

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
	} else if arg == "-v" || arg == "--version" || arg == "version" {
		fmt.Println("intime - v0.1")
	} else {
		Usage()
	}
}
