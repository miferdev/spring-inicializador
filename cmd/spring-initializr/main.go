package main

import "fmt"

// version is overridden at build time with:
//
//	go build -ldflags "-X main.version=<version>" ./cmd/spring-initializr
var version = "dev"

func main() {
	fmt.Printf("spring-initializr %s\n", version)
}
