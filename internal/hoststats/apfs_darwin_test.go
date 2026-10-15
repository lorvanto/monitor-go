//go:build darwin

package hoststats

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestDarwinAPFSCommandHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runAPFSCommand(ctx, "apfs", "list", "-plist"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled diskutil command: %v", err)
	}
}

func TestDarwinAPFSCommandOutputIsBoundedEvenThroughIOCopy(t *testing.T) {
	var output boundedAPFSOutput
	// os/exec copies a pipe into the output writer; embedding bytes.Buffer
	// would accidentally expose ReadFrom and bypass the size-limited Write.
	if _, err := io.Copy(&output, bytes.NewReader(make([]byte, 4<<20))); err != nil {
		t.Fatalf("valid output rejected: %v", err)
	}
	if _, err := io.Copy(&output, bytes.NewReader([]byte("x"))); err == nil || output.buffer.Len() != 4<<20 {
		t.Fatal("diskutil output limit bypassed")
	}
}
