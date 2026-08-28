// Package ops maps a plugin operation name onto the work it performs.
package ops

import (
	"fmt"

	"github.com/gokal/stash-awards/internal/protocol"
)

// Dispatch runs the operation named by the "mode" argument.
func Dispatch(log *protocol.Log, in protocol.Input) (interface{}, error) {
	mode := in.Args.String("mode")
	if mode == "" {
		return nil, fmt.Errorf("no mode argument supplied")
	}

	switch mode {
	case "ping":
		// Lets the UI confirm the backend binary is installed and runnable.
		return map[string]interface{}{"ok": true, "pluginDir": in.ServerConnection.PluginDir}, nil
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
}
