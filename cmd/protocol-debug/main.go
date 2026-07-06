package main

import (
	"os"

	"github.com/mahiro424/cbs/internal/protocoldebug"
)

func main() {
	os.Exit(protocoldebug.Run(os.Args[1:], os.Stdout, os.Stderr))
}
