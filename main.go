package main

import (
	"fmt"
	"os"

	"video_processing/internal/processor"
)

func main() {
	proc := processor.New()

	if err := proc.Run(); err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}
}
