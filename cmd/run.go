//(c) 2026 Chris Delezenski <chris.delezenski@gmail.com>
// This code is licensed under Apache 2.0 license (see LICENSE for details)

package main

import (
	"log"
	"os"
	"path/filepath"
)

func main() {
	name := filepath.Base(os.Args[0])

	switch name {
	case "ctrqctl":
		RunCLI()
	case "ctrq-service":
		RunServices()
	default:
		log.Fatalf("unknown command name %q", name)
	}
}
