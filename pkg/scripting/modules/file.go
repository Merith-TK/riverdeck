package modules

import (
	"os"
	"path/filepath"

	lua "github.com/yuin/gopher-lua"
)

// checkFileAccess returns true if path is within the config directory.
func checkFileAccess(path string, L *lua.LState) bool {
	configDir := L.GetGlobal("CONFIG_DIR").String()
	if configDir == "" {
		return true
	}
	return filepath.HasPrefix(filepath.Clean(path), filepath.Clean(configDir))
}

// FileModule provides file system operations for Lua scripts.
type FileModule struct{}

// NewFileModule creates a new file module.
func NewFileModule() *FileModule {
	return &FileModule{}
}

// Loader returns the Lua module loader function.
func (m *FileModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"read":   m.fileRead,
		"write":  m.fileWrite,
		"append": m.fileAppend,
		"exists": m.fileExists,
		"mkdir":  m.fileMkdir,
		"list":   m.fileList,
		"remove": m.fileRemove,
		"size":   m.fileSize,
		"is_dir": m.fileIsDir,
	})
	L.Push(mod)
	return 1
}

func (m *FileModule) fileRead(L *lua.LState) int {
	path := L.CheckString(1)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LNil)
		L.Push(lua.LString("access denied"))
		return 2
	}

	data, err := os.ReadFile(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}

func (m *FileModule) fileWrite(L *lua.LState) int {
	path := L.CheckString(1)
	content := L.CheckString(2)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LFalse)
		L.Push(lua.LString("access denied"))
		return 2
	}

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func (m *FileModule) fileAppend(L *lua.LState) int {
	path := L.CheckString(1)
	content := L.CheckString(2)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LFalse)
		L.Push(lua.LString("access denied"))
		return 2
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func (m *FileModule) fileExists(L *lua.LState) int {
	path := L.CheckString(1)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LFalse)
		return 1
	}

	_, err := os.Stat(path)
	L.Push(lua.LBool(err == nil))
	return 1
}

func (m *FileModule) fileMkdir(L *lua.LState) int {
	path := L.CheckString(1)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LFalse)
		L.Push(lua.LString("access denied"))
		return 2
	}

	err := os.MkdirAll(path, 0755)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func (m *FileModule) fileList(L *lua.LState) int {
	path := L.CheckString(1)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LNil)
		L.Push(lua.LString("access denied"))
		return 2
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	for i, entry := range entries {
		entryTbl := L.NewTable()
		entryTbl.RawSetString("name", lua.LString(entry.Name()))
		entryTbl.RawSetString("is_dir", lua.LBool(entry.IsDir()))
		// DirEntry.Info() is a cheap cached call for local filesystems
		if info, err := entry.Info(); err == nil {
			entryTbl.RawSetString("size", lua.LNumber(info.Size()))
		} else {
			entryTbl.RawSetString("size", lua.LNumber(0))
		}
		tbl.RawSetInt(i+1, entryTbl)
	}

	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func (m *FileModule) fileRemove(L *lua.LState) int {
	path := L.CheckString(1)

	// Check file access permissions
	if !checkFileAccess(path, L) {
		L.Push(lua.LFalse)
		L.Push(lua.LString("access denied"))
		return 2
	}

	err := os.Remove(path)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

func (m *FileModule) fileSize(L *lua.LState) int {
	path := L.CheckString(1)

	if !checkFileAccess(path, L) {
		L.Push(lua.LNumber(-1))
		return 1
	}

	info, err := os.Stat(path)
	if err != nil {
		L.Push(lua.LNumber(-1))
		return 1
	}

	L.Push(lua.LNumber(info.Size()))
	return 1
}

func (m *FileModule) fileIsDir(L *lua.LState) int {
	path := L.CheckString(1)

	if !checkFileAccess(path, L) {
		L.Push(lua.LFalse)
		return 1
	}

	info, err := os.Stat(path)
	if err != nil {
		L.Push(lua.LFalse)
		return 1
	}

	L.Push(lua.LBool(info.IsDir()))
	return 1
}
