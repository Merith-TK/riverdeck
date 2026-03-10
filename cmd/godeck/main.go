package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	for {
		app := NewApp()

		if err := app.Init(); err != nil {
			log.Fatal(err)
		}

		if err := app.Run(); err != nil {
			app.Shutdown()
			log.Fatal(err)
		}

		app.Shutdown()

		if !app.restartRequested {
			break
		}

		// Relaunch: re-exec the current binary with the same arguments.
		// Shutdown() above already closed the HID device so the new process
		// can claim it cleanly.
		exe, err := os.Executable()
		if err != nil {
			log.Printf("Restart: could not resolve executable: %v - falling back to in-process restart", err)
			continue
		}
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Printf("Restart: exec failed: %v - falling back to in-process restart", err)
			continue
		}
		// The new process is running; exit this one.
		os.Exit(0)
	}
}
