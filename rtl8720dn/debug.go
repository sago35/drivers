package rtl8720dn

import (
	"fmt"
)

var (
	debug = false
)

func Debug(b bool) {
	debug = b
}

func dbgPrintf(format string, a ...interface{}) {
	if debug {
		fmt.Printf(format, a...)
	}
}
