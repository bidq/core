package main

import (
	"time"

	"github.com/bidq/bidq/bidq"
)

func main() {
	bidq.Start(5000 * time.Millisecond)
}
