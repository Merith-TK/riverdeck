package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
)

var configDir = flag.String("configdir", "", "Configuration directory path")
var simAddr = flag.String("sim", "", "Connect to riverdeck-simulator at host:port instead of real hardware")

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	flag.Parse()

	for {
		app := NewApp()

		if err := app.Init(*configDir, *simAddr); err != nil {
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
