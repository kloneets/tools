package main

import (
	"os"

	"github.com/kloneets/tools/src/app"
)

func main() {
	if code, handled := app.RunCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	app.InitApp()
}
