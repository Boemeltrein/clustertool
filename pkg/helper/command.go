package helper

import "strings"

// ExtractNode returns the node passed to a generated talosctl command.
func ExtractNode(cmd string) string {
	parts := strings.Split(cmd, " ")
	for index, part := range parts {
		if strings.HasPrefix(part, "--nodes=") {
			return strings.TrimPrefix(part, "--nodes=")
		}
		if (part == "-n" || part == "--nodes") && index+1 < len(parts) {
			return parts[index+1]
		}
	}
	return ""
}
