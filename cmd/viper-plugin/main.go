// Command viper-plugin makes signing keys and signed plugin archives:
//
//	go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin keygen
//	go run github.com/Polyphron-AI/viper-plugin/cmd/viper-plugin sign --key plugin.key ./myplugin
package main

import (
	"fmt"
	"os"

	"github.com/Polyphron-AI/viper-plugin/archive"
)

// main runs the command and exits 1 on an error.
func main() {
	if err := archive.Command(
		"viper-plugin",
		os.Args[1:],
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
