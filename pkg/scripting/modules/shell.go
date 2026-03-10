package modules

import (
	"os/exec"
	"runtime"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ShellModule provides shell command execution for ScriptRunner.
type ShellModule struct{}

// NewShellModule creates a new shell module.
func NewShellModule() *ShellModule {
	return &ShellModule{}
}

// Loader returns the Lua module loader function.
func (m *ShellModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"exec":       m.shellExec,
		"exec_async": m.shellExecAsync,
		"open":       m.shellOpen,
		"terminal":   m.shellTerminal,
	})
	L.Push(mod)
	return 1
}

func (m *ShellModule) shellExec(L *lua.LState) int {
	cmdStr := L.CheckString(1)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	stdout, err := cmd.Output()
	exitCode := 0
	stderrStr := ""

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderrStr = string(exitErr.Stderr)
		} else {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			L.Push(lua.LNumber(-1))
			return 3
		}
	}

	L.Push(lua.LString(string(stdout)))
	L.Push(lua.LString(stderrStr))
	L.Push(lua.LNumber(exitCode))
	return 3
}

func (m *ShellModule) shellExecAsync(L *lua.LState) int {
	cmdStr := L.CheckString(1)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	err := cmd.Start()
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	go cmd.Wait()

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func (m *ShellModule) shellOpen(L *lua.LState) int {
	target := L.CheckString(1)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}

	err := cmd.Start()
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	go cmd.Wait()

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func (m *ShellModule) shellTerminal(L *lua.LState) int {
	cmdStr := L.CheckString(1)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// "start cmd /k <command>" opens a new cmd window and runs the command
		// Pass as single string to cmd /c to avoid argument parsing issues
		cmd = exec.Command("cmd", "/c", "start", cmdStr)
	case "darwin":
		// Use osascript to open Terminal and run command
		script := `tell application "Terminal" to do script "` + strings.ReplaceAll(cmdStr, `"`, `\"`) + `"`
		cmd = exec.Command("osascript", "-e", script)
	default:
		// Linux: try common terminal emulators
		terminals := [][]string{
			{"x-terminal-emulator", "-e"},
			{"gnome-terminal", "--"},
			{"konsole", "-e"},
			{"xfce4-terminal", "-e"},
			{"xterm", "-e"},
		}
		for _, term := range terminals {
			if _, err := exec.LookPath(term[0]); err == nil {
				args := append(term[1:], "sh", "-c", cmdStr)
				cmd = exec.Command(term[0], args...)
				break
			}
		}
		if cmd == nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString("no terminal emulator found"))
			return 2
		}
	}

	err := cmd.Start()
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	go cmd.Wait()

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}
