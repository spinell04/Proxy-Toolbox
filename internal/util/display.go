package util

// ShortenErr truncates an error message for display.
func ShortenErr(err error) string {
	s := err.Error()
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}
