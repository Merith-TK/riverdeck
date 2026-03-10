package modules

import (
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// HTTPModule provides HTTP functionality.
type HTTPModule struct {
	client *http.Client
}

// NewHTTPModule creates a new HTTP module.
func NewHTTPModule() *HTTPModule {
	// Create optimized HTTP client
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second, // Reduced from 30s for better responsiveness
	}

	return &HTTPModule{
		client: client,
	}
}

// Loader returns the Lua module loader function.
func (m *HTTPModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"get":     m.httpGet,
		"post":    m.httpPost,
		"request": m.httpRequest,
	})
	L.Push(mod)
	return 1
}

func (m *HTTPModule) httpGet(L *lua.LState) int {
	url := L.CheckString(1)

	resp, err := m.client.Get(url)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(string(body)))
	L.Push(lua.LNumber(resp.StatusCode))
	return 2
}

func (m *HTTPModule) httpPost(L *lua.LState) int {
	url := L.CheckString(1)
	contentType := L.CheckString(2)
	body := L.CheckString(3)

	resp, err := m.client.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(string(respBody)))
	L.Push(lua.LNumber(resp.StatusCode))
	return 2
}

func (m *HTTPModule) httpRequest(L *lua.LState) int {
	method := L.CheckString(1)
	url := L.CheckString(2)
	headers := L.OptTable(3, nil)
	body := L.OptString(4, "")
	// Optional 5th argument: timeout in milliseconds (0 = use client default).
	timeoutMs := L.OptInt(5, 0)

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	if headers != nil {
		headers.ForEach(func(key, value lua.LValue) {
			req.Header.Set(key.String(), value.String())
		})
	}

	// Use a per-request client with custom timeout when requested.
	client := m.client
	if timeoutMs > 0 {
		client = &http.Client{
			Transport: m.client.Transport,
			Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(string(respBody)))
	L.Push(lua.LNumber(resp.StatusCode))
	return 2
}
