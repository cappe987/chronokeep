//go:build noweb

package app

import "fmt"

func WebServer() {
	fmt.Printf("Web not included in build. Build without `noweb` tag\n")
}
