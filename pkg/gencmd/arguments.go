package gencmd

import (
	"fmt"
	"strings"
)

// Do not let forwarded flags change the validated node, input, identity, image,
// or endpoint. Those values have one source in the cluster configuration.
func ValidateExtraArgs(args []string) error {
	for _, arg := range args {
		flag, _, _ := strings.Cut(arg, "=")
		switch flag {
		case "-n", "--nodes", "-e", "--endpoints", "-f", "--file", "--talosconfig", "--context", "--image", "--to":
			return fmt.Errorf("%s must be configured in the cluster files, not forwarded as a flag", flag)
		}
	}
	return nil
}
