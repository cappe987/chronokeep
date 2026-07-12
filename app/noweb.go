//go:build noweb

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>

package app

import "fmt"

func WebServer() {
	fmt.Printf("Web not included in build. Build without `noweb` tag\n")
}
