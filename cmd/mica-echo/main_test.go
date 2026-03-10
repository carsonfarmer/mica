package main

import (
	"os"
	"testing"
	"time"
)

func TestMainExitsOnClosedInput(t *testing.T) {
	oldIn := os.Stdin
	oldOut := os.Stdout
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe stdin: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe stdout: %v", err)
	}
	os.Stdin = inR
	os.Stdout = outW
	t.Cleanup(func() {
		os.Stdin = oldIn
		os.Stdout = oldOut
		_ = inR.Close()
		_ = inW.Close()
		_ = outR.Close()
		_ = outW.Close()
	})

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	_ = inW.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("main did not exit after stdin closed")
	}
}
