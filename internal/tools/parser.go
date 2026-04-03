package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"

	"proxytoolbox/internal/proxy"
)

var proxyFormats = []struct {
	Name    string
	Example string
	Format  func(p proxy.Proxy) string
}{
	{"host:port:user:pass", "1.2.3.4:8080:admin:secret", func(p proxy.Proxy) string {
		return fmt.Sprintf("%s:%s:%s:%s", p.Host, p.Port, p.User, p.Password)
	}},
	{"user:pass:host:port", "admin:secret:1.2.3.4:8080", func(p proxy.Proxy) string {
		return fmt.Sprintf("%s:%s:%s:%s", p.User, p.Password, p.Host, p.Port)
	}},
	{"user:pass@host:port", "admin:secret@1.2.3.4:8080", func(p proxy.Proxy) string {
		return fmt.Sprintf("%s:%s@%s:%s", p.User, p.Password, p.Host, p.Port)
	}},
	{"http://user:pass@host:port", "http://admin:secret@1.2.3.4:8080", func(p proxy.Proxy) string {
		return fmt.Sprintf("http://%s:%s@%s:%s", p.User, p.Password, p.Host, p.Port)
	}},
	{"host:port (no auth)", "1.2.3.4:8080", func(p proxy.Proxy) string {
		return fmt.Sprintf("%s:%s", p.Host, p.Port)
	}},
}

func RunParser() {
	filePath, err := proxy.SelectFile()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	raw, err := proxy.LoadRawLines(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	var proxies []proxy.Proxy
	for _, line := range raw {
		if p, ok := proxy.ParseLine(line); ok {
			proxies = append(proxies, p)
		}
	}
	if len(proxies) == 0 {
		fmt.Println("No valid proxies found.")
		return
	}

	var formatIdx int
	var options []huh.Option[int]
	for i, f := range proxyFormats {
		options = append(options, huh.NewOption(fmt.Sprintf("%-30s  e.g. %s", f.Name, f.Example), i))
	}
	err = huh.NewSelect[int]().
		Title("Select output format").
		Options(options...).
		Value(&formatIdx).
		Run()
	if err != nil {
		fmt.Println("Selection cancelled.")
		return
	}

	chosen := proxyFormats[formatIdx]
	var lines []string
	for _, p := range proxies {
		lines = append(lines, chosen.Format(p))
	}

	output := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(output), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	fmt.Printf("\nConverted %d proxies to %s\n", len(proxies), chosen.Name)
	fmt.Printf("File: %s\n", filePath)
}
