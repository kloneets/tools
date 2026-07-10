package helpers

type GlobalOptions struct {
	Debug bool
}

var goInstance *GlobalOptions

func CurrentGlobals() *GlobalOptions {
	return goInstance
}

func Globals() *GlobalOptions {
	if goInstance == nil {
		return InitGlobals()
	}
	return goInstance
}

func InitGlobals() *GlobalOptions {
	goInstance = &GlobalOptions{Debug: false}
	return goInstance
}

func SetMainWindow(any) {}

func SetMainOverlay(any) {}
