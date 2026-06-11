package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "stratum-agent MVP stub: process supervision is not started yet")
	// TODO: authenticate with the controller and supervise MCDR through the agent runtime boundary.
}
