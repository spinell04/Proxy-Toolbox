package tools

import (
	"fmt"
	"math/rand"
	"os"
	"strings"

	"proxytoolbox/internal/proxy"
)

func RunRandomizer() {
	filePath, err := proxy.SelectFile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	lines, err := proxy.LoadRawLines(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	if len(lines) == 0 {
		fmt.Println("No proxy lines found in file.")
		return
	}

	fmt.Printf("\nFile    : %s\n", filePath)
	fmt.Printf("Proxies : %d\n", len(lines))
	fmt.Print("Shuffling... ")

	rand.Shuffle(len(lines), func(i, j int) {
		lines[i], lines[j] = lines[j], lines[i]
	})

	output := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(output), 0644); err != nil {
		fmt.Printf("\nError writing file: %v\n", err)
		return
	}

	fmt.Printf("done.\n")
	fmt.Printf("\nShuffled %d proxies in %s\n", len(lines), filePath)
}
