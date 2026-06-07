package utils

import (
	"os"
	"strings"
)

// GetHomeDir returns the user's home directory.
func GetHomeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// AnonymizeLogContent replaces sensitive information in log content.
func AnonymizeLogContent(content string, sensitivePaths ...string) string {
	home := GetHomeDir()
	if home != "" {
		sensitivePaths = append(sensitivePaths, home)
	}

	for _, p := range sensitivePaths {
		if p == "" || p == "/" || p == "." {
			continue
		}
		
		placeholder := "{{SENSITIVE_PATH}}"
		if p == home {
			placeholder = "{{USER_HOME}}"
		}

		// Replace path with placeholder
		content = strings.ReplaceAll(content, p, placeholder)
		
		// Also handle cases where slashes might be escaped or different
		altPath := strings.ReplaceAll(p, "\\", "/")
		if altPath != p {
			content = strings.ReplaceAll(content, altPath, placeholder)
		}
		
		altPath2 := strings.ReplaceAll(p, "/", "\\")
		if altPath2 != p {
			content = strings.ReplaceAll(content, altPath2, placeholder)
		}
	}

	return content
}
