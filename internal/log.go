// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
package internal

import (
	"fmt"
	"log"
	"time"
)

var (
	traceLevel  *log.Logger
	debugLevel  *log.Logger
	infoLevel   *log.Logger
	noticeLevel *log.Logger
	warnLevel   *log.Logger
	errorLevel  *log.Logger
	fatalLevel  *log.Logger
	loglevel    int = 4
)

func setLogLevel(level string) {
	switch level {
	case "trace":
		loglevel = 6
	case "debug":
		loglevel = 5
	case "info":
		loglevel = 4
	case "notice":
		loglevel = 3
	case "warn":
		loglevel = 2
	case "error":
		loglevel = 1
	case "fatal":
		loglevel = 0
	default:
		LogFatal("Invalid loglevel")
	}
}

type logWriter struct {
	lvl string
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(time.Now().UTC().Format(
		"2006-01-02 15:04:05") + " [" + writer.lvl + "] " + string(bytes))
}

func init() {
	traceLevel = log.New(logWriter{lvl: "TRACE"}, "", 0)
	debugLevel = log.New(logWriter{lvl: "DEBUG"}, "", 0)
	infoLevel = log.New(logWriter{lvl: "INFO"}, "", 0)
	noticeLevel = log.New(logWriter{lvl: "NOTICE"}, "", 0)
	warnLevel = log.New(logWriter{lvl: "WARNING"}, "", 0)
	errorLevel = log.New(logWriter{lvl: "ERROR"}, "", 0)
	fatalLevel = log.New(logWriter{lvl: "FATAL"}, "", 0)
}

func LogTrace(s string, args ...any) {
	if loglevel >= 6 {
		traceLevel.Printf(s, args...)
	}
}
func LogDebug(s string, args ...any) {
	if loglevel >= 5 {
		debugLevel.Printf(s, args...)
	}
}
func LogInfo(s string, args ...any) {
	if loglevel >= 4 {
		infoLevel.Printf(s, args...)
	}
}
func LogNotice(s string, args ...any) {
	if loglevel >= 3 {
		infoLevel.Printf(s, args...)
	}
}
func LogWarn(s string, args ...any) {
	if loglevel >= 2 {
		warnLevel.Printf(s, args...)
	}
}
func LogError(s string, args ...any) {
	if loglevel >= 1 {
		errorLevel.Printf(s, args...)
	}
}
func LogFatal(s string, args ...any) {
	fatalLevel.Printf(s, args...)
	panic("Fatal error")
}
func (port *Port) CliPrint(s string, args ...any) {
	if port.Silent {
		return
	}
	if port.App.Cli {
		fmt.Printf(s, args...)
	}
}
