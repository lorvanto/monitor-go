//go:build darwin

package hoststats

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

type boundedAPFSOutput struct {
	buffer bytes.Buffer
}

func (output *boundedAPFSOutput) Write(data []byte) (int, error) {
	if output.buffer.Len()+len(data) > 4<<20 {
		return 0, errors.New("diskutil output exceeds 4 MiB")
	}
	return output.buffer.Write(data)
}

func platformAPFSCommand() apfsCommandFunc {
	return runAPFSCommand
}

func runAPFSCommand(ctx context.Context, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/sbin/diskutil", arguments...)
	command.WaitDelay = 200 * time.Millisecond
	var output boundedAPFSOutput
	command.Stdout = &output
	// Do not include stderr in reports: disk labels and tool diagnostics can
	// contain unrelated device details. The operation and exit status suffice.
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return output.buffer.Bytes(), nil
}
