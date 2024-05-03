package helpers

import "log"

type GlobalOptions struct {
	Debug bool
}

var goInstance *GlobalOptions

func Globals() *GlobalOptions {
	if goInstance == nil {
		log.Fatal("We have no global options!")
	}

	return goInstance
}

func InitGlobals() *GlobalOptions {
	goInstance = &GlobalOptions{
		Debug: false,
	}

	return goInstance
}
