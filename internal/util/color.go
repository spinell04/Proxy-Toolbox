package util

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
)

var colorEnabled = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

func Red(s string) string {
	if !colorEnabled {
		return s
	}
	return fmt.Sprintf("\033[31m%s\033[0m", s)
}

func Green(s string) string {
	if !colorEnabled {
		return s
	}
	return fmt.Sprintf("\033[32m%s\033[0m", s)
}

func Yellow(s string) string {
	if !colorEnabled {
		return s
	}
	return fmt.Sprintf("\033[33m%s\033[0m", s)
}
