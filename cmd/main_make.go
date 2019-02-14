// +build make

package main

import (
	"fmt"
	"time"
)

// This is a quick hack for being able to compile in the compiled date,
// As OSX date command can't easily produce the format expected from Go
// This is not used in regular running of the program, only as a pre-step to compilation.
func main() {
	fmt.Println(time.Now().Format(time.RFC3339))
}
