package main

import (
	"interview-project/internal/cli"
	"os"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
