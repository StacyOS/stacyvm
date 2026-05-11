//go:build !linux

package main

import "fmt"

func main() {
	fmt.Println("stacyvm-agent only runs on Linux")
}
