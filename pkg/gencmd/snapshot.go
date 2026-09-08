package gencmd

import (
	"fmt"
	"github.com/trueforge-org/clustertool/pkg/helper"
	"os"
	"path/filepath"
)

// Copy every planned input before executing the first node. Another genconfig
// invocation cannot replace a later node's input midway through this operation.
func freezeCommands(commands []Command) ([]Command, func(), error) {
	directory := ""
	cleanup := func() {
		if directory != "" {
			_ = os.RemoveAll(directory)
		}
	}
	frozen := append([]Command{}, commands...)
	files := map[string]string{}
	for index, cmd := range frozen {
		if cmd.Err != nil {
			cleanup()
			return nil, func() {}, cmd.Err
		}
		if !cmd.Snapshot {
			continue
		}
		if directory == "" {
			var err error
			directory, err = os.MkdirTemp(helper.TalosPath, ".execute-")
			if err != nil {
				return nil, cleanup, err
			}
		}
		frozen[index].Args = append([]string{}, cmd.Args...)
		for i := 0; i+1 < len(cmd.Args); i++ {
			if cmd.Args[i] != "-f" && cmd.Args[i] != "--talosconfig" {
				continue
			}
			source := cmd.Args[i+1]
			dest, ok := files[source]
			if !ok {
				data, err := os.ReadFile(source)
				if err != nil {
					cleanup()
					return nil, func() {}, fmt.Errorf("freeze command input %s: %w", source, err)
				}
				dest = filepath.Join(directory, fmt.Sprintf("%d-%s", len(files), filepath.Base(source)))
				if err := os.WriteFile(dest, data, 0600); err != nil {
					cleanup()
					return nil, func() {}, err
				}
				files[source] = dest
			}
			frozen[index].Args[i+1] = dest
		}
	}
	return frozen, cleanup, nil
}
