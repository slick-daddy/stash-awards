// Command stash-awards is the backend half of the Stash Awards plugin. Stash
// spawns it once per operation, writes a JSON request on stdin, and reads a
// JSON reply from stdout; log and progress lines go to stderr.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/slick-daddy/stash-awards/internal/ops"
	"github.com/slick-daddy/stash-awards/internal/protocol"
)

// version is stamped by the release build (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	log := protocol.NewLog()

	var out protocol.Output
	if err := run(log, &out); err != nil {
		out.SetError(err)
		log.Error("%v", err)
	}

	// Stash parses stdout as the operation result, so nothing else may be
	// written there.
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		log.Error("could not encode plugin output: %v", err)
		os.Exit(1)
	}
}

func run(log *protocol.Log, out *protocol.Output) error {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		out.Output = version
		return nil
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	if len(raw) == 0 {
		return errors.New("no plugin input on stdin: this program is run by Stash, not directly")
	}

	var in protocol.Input
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("decoding plugin input: %w", err)
	}

	result, err := ops.Dispatch(log, in)
	if err != nil {
		return err
	}
	out.Output = result
	return nil
}
