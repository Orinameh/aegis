package banner

import (
	"fmt"
	"os"
	"strings"
)

// Style defines the banner style
type Style string

const (
	StyleSimple  Style = "simple"
	StyleMinimal Style = "minimal"
	StyleColor   Style = "color"
	StyleNone    Style = "none"
)

// Config holds banner configuration
type Config struct {
	Style    Style
	Version  string
	Color    bool
	Suppress bool
}

// NewConfig creates a default banner config
func NewConfig(version string) *Config {
	return &Config{
		Style:    StyleSimple,
		Version:  version,
		Color:    false,
		Suppress: false,
	}
}

// Print prints the banner
func (c *Config) Print() {
	if c.Suppress {
		return
	}

	var banner string
	switch c.Style {
	case StyleSimple:
		banner = getSimpleBanner(c.Version)
	case StyleMinimal:
		banner = getMinimalBanner(c.Version)
	case StyleColor:
		banner = getColorBanner(c.Version)
	case StyleNone:
		return
	default:
		banner = getSimpleBanner(c.Version)
	}

	fmt.Print(banner)
}

// getSimpleBanner returns the simple ASCII banner
func getSimpleBanner(version string) string {
	return fmt.Sprintf(`
   ╔═══════════════════════════════════════════╗
   ║                                         ║
   ║    █████╗ ███████╗ ██████╗ ██╗███████╗  ║
   ║   ██╔══██╗██╔════╝██╔════╝ ██║██╔════╝  ║
   ║   ███████║█████╗  ██║  ███╗██║███████╗  ║
   ║   ██╔══██║██╔══╝  ██║   ██║██║╚════██║  ║
   ║   ██║  ██║███████╗╚██████╔╝██║███████║  ║
   ║   ╚═╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝╚══════╝  ║
   ║                                         ║
   ║   Protected Infrastructure Cleaning     ║
   ║              v%s                        ║
   ╚═══════════════════════════════════════════╝
`, version)
}

// getMinimalBanner returns the minimal banner
func getMinimalBanner(version string) string {
	width := 55
	line := strings.Repeat("═", width)

	return fmt.Sprintf(`
╔%s╗
║  🛡️  AEGIS v%-30s ║
║  Protected Infrastructure Cleaning      ║
╚%s╝
`, line, version, line)
}

// getColorBanner returns the colorized banner
func getColorBanner(version string) string {
	return fmt.Sprintf(`
\033[38;5;208m   ╔═══════════════════════════════════════════╗
   ║                                         ║
   ║    █████╗ ███████╗ ██████╗ ██╗███████╗  ║
   ║   ██╔══██╗██╔════╝██╔════╝ ██║██╔════╝  ║
   ║   ███████║█████╗  ██║  ███╗██║███████╗  ║
   ║   ██╔══██║██╔══╝  ██║   ██║██║╚════██║  ║
   ║   ██║  ██║███████╗╚██████╔╝██║███████║  ║
   ║   ╚═╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝╚══════╝  ║
   ║                                         ║
   ║   \033[38;5;39mProtected Infrastructure Cleaning\033[0m    ║
   ║              \033[38;5;226mv%s\033[0m                        ║
   ╚═══════════════════════════════════════════╝\033[0m
`, version)
}

// SupportsColor checks if the terminal supports color
func SupportsColor() bool {
	// Check if we're in a terminal that supports color
	// For simplicity, we'll check if the TERM environment variable exists
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}
