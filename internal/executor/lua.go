package executor

import (
	"context"
	"fmt"
	"homescript-server/internal/logger"
	"homescript-server/internal/storage"
	"homescript-server/internal/types"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cjoudrey/gluahttp"
	"github.com/nubix-io/gluasocket"
	lua "github.com/yuin/gopher-lua"
)

// luaStateTracker tracks reference count for Lua states
type luaStateTracker struct {
	state      *lua.LState
	refCount   int
	mutex      sync.Mutex
	executeMux sync.Mutex // Protects concurrent execution in the same state
}

// Executor manages Lua script execution
type Executor struct {
	storage       *storage.Storage
	deviceManager DeviceManager
	scheduler     interface{} // Scheduler interface to avoid circular dependency
	scriptTimeout time.Duration
	configPath    string // Base path for config directory
	stateTrackers map[*lua.LState]*luaStateTracker
	trackersMutex sync.RWMutex
}

// DeviceManager interface for device operations
type DeviceManager interface {
	Get(id string) (map[string]interface{}, error)
	Set(id string, attrs map[string]interface{}) error
}

// New creates a new Executor
func New(store *storage.Storage, dm DeviceManager, configPath string) *Executor {
	return &Executor{
		storage:       store,
		deviceManager: dm,
		scheduler:     nil,
		scriptTimeout: 5 * time.Second,
		configPath:    configPath,
		stateTrackers: make(map[*lua.LState]*luaStateTracker),
	}
}

// SetScheduler sets the scheduler reference (called after scheduler is created)
func (e *Executor) SetScheduler(sched interface{}) {
	e.scheduler = sched
}

// addStateReference adds a reference to a Lua state
func (e *Executor) addStateReference(L *lua.LState) {
	e.trackersMutex.Lock()
	defer e.trackersMutex.Unlock()

	tracker, exists := e.stateTrackers[L]
	if !exists {
		tracker = &luaStateTracker{
			state:    L,
			refCount: 0,
		}
		e.stateTrackers[L] = tracker
	}

	tracker.mutex.Lock()
	tracker.refCount++
	tracker.mutex.Unlock()

	logger.Debug("Lua state %p reference count increased to %d", L, tracker.refCount)
}

// releaseStateReference releases a reference to a Lua state and closes it if no more references
func (e *Executor) releaseStateReference(L *lua.LState) {
	e.trackersMutex.Lock()
	defer e.trackersMutex.Unlock()

	tracker, exists := e.stateTrackers[L]
	if !exists {
		logger.Warn("Attempting to release non-tracked Lua state %p", L)
		return
	}

	tracker.mutex.Lock()
	tracker.refCount--
	count := tracker.refCount
	tracker.mutex.Unlock()

	logger.Debug("Lua state %p reference count decreased to %d", L, count)

	if count <= 0 {
		// Check if there are active timers before closing
		timerCount := L.GetGlobal("__timer_count__")
		activeTimers := 0
		if timerCount != lua.LNil {
			if num, ok := timerCount.(lua.LNumber); ok {
				activeTimers = int(num)
			}
		}

		if activeTimers > 0 {
			logger.Debug("Lua state %p has %d active timer(s), keeping tracker alive", L, activeTimers)
			// Don't close or remove from tracking yet - timers still need it
			return
		}

		L.Close()
		delete(e.stateTrackers, L)
		logger.Debug("Lua state %p closed and removed from tracking", L)
	}
}

// ReleaseStateReference is public method for releasing Lua state reference (used by scheduler)
func (e *Executor) ReleaseStateReference(L *lua.LState) {
	e.releaseStateReference(L)
}

// Execute runs a Lua script with the given event
func (e *Executor) Execute(scriptPath string, event *types.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.scriptTimeout)
	defer cancel()

	L := lua.NewState()

	// Track this state with initial reference count of 1
	e.addStateReference(L)

	// NOTE: We do NOT defer release here!
	// If timers are created, they will hold references and release when done.
	// If no timers created, we need to release manually after script completes.
	shouldRelease := true
	defer func() {
		if shouldRelease {
			e.releaseStateReference(L)
		}
	}()

	// Set timeout context
	L.SetContext(ctx)

	// Add config/lib to Lua package path for helper libraries
	libPath := filepath.Join(e.configPath, "lib")
	configLibPath := fmt.Sprintf("%s/?.lua;%s/?/init.lua", libPath, libPath)
	if err := L.DoString(fmt.Sprintf(`package.path = package.path .. ";%s"`, configLibPath)); err != nil {
		logger.Warn("Failed to set Lua package path: %v", err)
	}

	// Preload color helpers
	if err := L.DoString(`color = require("color_helpers")`); err != nil {
		logger.Warn("Failed to load color helpers: %v", err)
	}

	// Preload Frigate helpers
	if err := L.DoString(`frigate = require("frigate_helpers")`); err != nil {
		logger.Warn("Failed to load Frigate helpers: %v", err)
	}

	// Load socket library for network operations (HTTP, TCP, UDP)
	gluasocket.Preload(L)
	L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)

	// Set SCRIPT_DIR global variable (directory of current script)
	scriptDir := filepath.Dir(scriptPath)
	L.SetGlobal("SCRIPT_DIR", lua.LString(scriptDir))

	// Set SCRIPT_PATH global variable (full path to current script)
	L.SetGlobal("SCRIPT_PATH", lua.LString(scriptPath))

	// Register API functions
	e.registerAPI(L, event)

	// Register DoSiblings helper
	e.registerDoSiblings(L, scriptPath, event)

	// Execute script
	if err := L.DoFile(scriptPath); err != nil {
		return fmt.Errorf("script execution failed: %w", err)
	}

	// Check if any timers were created during script execution
	// If timers were created, they now own the reference - don't release here
	timersCreated := L.GetGlobal("__timers_created__")
	if timersCreated != lua.LNil && lua.LVAsBool(timersCreated) {
		shouldRelease = false
		logger.Debug("Lua state %p has active timers, not releasing from Execute", L)
	}

	return nil
}

// ExecuteCallback runs a serialized Lua callback function
func (e *Executor) ExecuteCallback(callback *lua.LFunction, L *lua.LState, timerID string) error {
	// Safety checks
	if callback == nil {
		return fmt.Errorf("callback is nil for timer %s", timerID)
	}
	if L == nil {
		return fmt.Errorf("lua state is nil for timer %s", timerID)
	}

	// Lock the state to prevent concurrent access from multiple timer goroutines
	e.trackersMutex.RLock()
	tracker, exists := e.stateTrackers[L]
	e.trackersMutex.RUnlock()

	if !exists {
		return fmt.Errorf("lua state %p not tracked for timer %s", L, timerID)
	}

	// Acquire execution lock for this state
	tracker.executeMux.Lock()
	defer tracker.executeMux.Unlock()

	logger.Debug("Timer %s acquired lock on Lua state %p", timerID, L)

	ctx, cancel := context.WithTimeout(context.Background(), e.scriptTimeout)
	defer cancel()

	// Set timeout context
	L.SetContext(ctx)

	err := L.CallByParam(lua.P{
		Fn:      callback,
		NRet:    0,
		Protect: true,
	})

	if err != nil {
		return fmt.Errorf("timer %s callback error: %w", timerID, err)
	}

	logger.Debug("Timer %s released lock on Lua state %p", timerID, L)

	return nil
}

func (e *Executor) registerAPI(L *lua.LState, event *types.Event) {
	// Event table
	eventTable := L.NewTable()
	eventTable.RawSetString("source", lua.LString(event.Source))
	eventTable.RawSetString("type", lua.LString(event.Type))
	if event.Device != "" {
		eventTable.RawSetString("device", lua.LString(event.Device))
	}
	if event.Attribute != "" {
		eventTable.RawSetString("attribute", lua.LString(event.Attribute))
	}
	if event.Topic != "" {
		eventTable.RawSetString("topic", lua.LString(event.Topic))
	}

	// Event data - convert all values properly
	dataTable := L.NewTable()
	for k, v := range event.Data {
		dataTable.RawSetString(k, e.toLuaValue(L, v))
	}
	eventTable.RawSetString("data", dataTable)
	L.SetGlobal("event", eventTable)

	// State API
	stateTable := L.NewTable()
	L.SetField(stateTable, "get", L.NewFunction(e.stateGet))
	L.SetField(stateTable, "set", L.NewFunction(e.stateSet))
	L.SetField(stateTable, "delete", L.NewFunction(e.stateDelete))
	L.SetGlobal("state", stateTable)

	// Device API
	deviceTable := L.NewTable()
	L.SetField(deviceTable, "get", L.NewFunction(e.deviceGet))
	L.SetField(deviceTable, "set", L.NewFunction(e.deviceSet))
	L.SetField(deviceTable, "call", L.NewFunction(e.deviceCall))
	L.SetGlobal("device", deviceTable)

	// Log functions
	logTable := L.NewTable()
	L.SetField(logTable, "info", L.NewFunction(e.logInfo))
	L.SetField(logTable, "warn", L.NewFunction(e.logWarn))
	L.SetField(logTable, "error", L.NewFunction(e.logError))
	L.SetGlobal("log", logTable)

	// Timer functions
	timerTable := L.NewTable()
	L.SetField(timerTable, "after", L.NewFunction(e.timerAfter))
	L.SetField(timerTable, "at", L.NewFunction(e.timerAt))
	L.SetField(timerTable, "every", L.NewFunction(e.timerEvery))
	L.SetField(timerTable, "cancel", L.NewFunction(e.timerCancel))
	L.SetField(timerTable, "list", L.NewFunction(e.timerList))
	L.SetGlobal("timer", timerTable)

	// UDP send function
	udpTable := L.NewTable()
	L.SetField(udpTable, "send", L.NewFunction(e.udpSend))
	L.SetGlobal("udp", udpTable)
}

// registerDoSiblings registers the DoSiblings helper function
func (e *Executor) registerDoSiblings(L *lua.LState, currentScript string, event *types.Event) {
	// Create closure with current script path and event
	doSiblings := L.NewFunction(func(L *lua.LState) int {
		scriptDir := filepath.Dir(currentScript)
		currentScriptName := filepath.Base(currentScript)

		// Read directory
		entries, err := os.ReadDir(scriptDir)
		if err != nil {
			logger.Error("DoSiblings: failed to read directory %s: %v", scriptDir, err)
			L.Push(lua.LFalse)
			return 1
		}

		executed := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Skip non-Lua files
			if !strings.HasSuffix(name, ".lua") {
				continue
			}

			// Skip current script (avoid infinite recursion)
			if name == currentScriptName {
				continue
			}

			siblingPath := filepath.Join(scriptDir, name)
			logger.Debug("DoSiblings: executing %s", siblingPath)

			// Execute sibling script with same event
			if err := e.Execute(siblingPath, event); err != nil {
				logger.Error("DoSiblings: failed to execute %s: %v", siblingPath, err)
			} else {
				executed++
			}
		}

		logger.Debug("DoSiblings: executed %d sibling scripts", executed)
		L.Push(lua.LNumber(executed))
		return 1
	})

	L.SetGlobal("DoSiblings", doSiblings)
}

// State functions
func (e *Executor) stateGet(L *lua.LState) int {
	key := L.CheckString(1)
	value, err := e.storage.Get(key)
	if err != nil {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(e.toLuaValue(L, value))
	return 1
}

func (e *Executor) stateSet(L *lua.LState) int {
	key := L.CheckString(1)
	value := e.fromLuaValue(L.Get(2))
	if err := e.storage.Set(key, value); err != nil {
		logger.Error("Failed to set state %s: %v", key, err)
	}
	return 0
}

func (e *Executor) stateDelete(L *lua.LState) int {
	key := L.CheckString(1)
	if err := e.storage.Delete(key); err != nil {
		logger.Error("Failed to delete state %s: %v", key, err)
	}
	return 0
}

// Device functions
func (e *Executor) deviceGet(L *lua.LState) int {
	id := L.CheckString(1)
	attrs, err := e.deviceManager.Get(id)
	if err != nil {
		logger.Error("Failed to get device %s: %v", id, err)
		L.Push(lua.LNil)
		return 1
	}

	table := L.NewTable()
	for k, v := range attrs {
		table.RawSetString(k, e.toLuaValue(L, v))
	}
	L.Push(table)
	return 1
}

func (e *Executor) deviceSet(L *lua.LState) int {
	id := L.CheckString(1)
	attrsTable := L.CheckTable(2)

	attrs := make(map[string]interface{})
	attrsTable.ForEach(func(key, value lua.LValue) {
		if keyStr, ok := key.(lua.LString); ok {
			attrs[string(keyStr)] = e.fromLuaValue(value)
		}
	})

	if err := e.deviceManager.Set(id, attrs); err != nil {
		logger.Error("Failed to set device %s: %v", id, err)
	}
	return 0
}

func (e *Executor) deviceCall(L *lua.LState) int {
	id := L.CheckString(1)
	action := L.CheckString(2)

	params := make(map[string]interface{})
	if L.GetTop() >= 3 {
		paramsTable := L.CheckTable(3)
		paramsTable.ForEach(func(key, value lua.LValue) {
			if keyStr, ok := key.(lua.LString); ok {
				params[string(keyStr)] = e.fromLuaValue(value)
			}
		})
	}

	// Execute action script directly: config/events/device/{id}/actions/{action}.lua
	scriptPath := filepath.Join(e.configPath, "events", "device", id, "actions", action+".lua")

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		logger.Error("Action script not found: %s (device: %s, action: %s)", scriptPath, id, action)
		L.Push(lua.LFalse)
		return 1
	}

	// Create action event
	event := &types.Event{
		Source:    "action",
		Type:      "call",
		Device:    id,
		Attribute: action,
		Data:      params,
		Timestamp: time.Now(),
	}

	// Execute action script in new Lua state
	if err := e.Execute(scriptPath, event); err != nil {
		logger.Error("Failed to execute action %s on %s: %v", action, id, err)
		L.Push(lua.LFalse)
		return 1
	}

	L.Push(lua.LTrue)
	return 1
}

// Log functions
func (e *Executor) logInfo(L *lua.LState) int {
	msg := L.CheckString(1)
	logger.Info("[LUA] %s", msg)
	return 0
}

func (e *Executor) logWarn(L *lua.LState) int {
	msg := L.CheckString(1)
	logger.Warn("[LUA] %s", msg)
	return 0
}

func (e *Executor) logError(L *lua.LState) int {
	msg := L.CheckString(1)
	logger.Error("[LUA] %s", msg)
	return 0
}

// UDP send function
// UdpSend(message, [host], [port])
// message — required: Lua string (can be text or binary, with \0 etc.)
// host    — optional, string, default "127.0.0.1"
// port    — optional, number, default 8125
// Returns: true/nil on success, false + error on failure
func (e *Executor) udpSend(L *lua.LState) int {
	// Check first argument — must be string (text or binary)
	if L.GetTop() < 1 {
		L.ArgError(1, "message expected")
		return 0
	}
	messageLua := L.CheckString(1) // Safe for binary strings!
	data := []byte(messageLua)     // Convert to []byte — works with any bytes including \0

	// Host (optional)
	host := "127.0.0.1"
	if L.GetTop() >= 2 {
		host = L.CheckString(2)
	}

	// Port (optional)
	port := 8125
	if L.GetTop() >= 3 {
		port = int(L.CheckNumber(3))
	}

	// Resolve address
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("resolve error: " + err.Error()))
		return 2
	}

	// Send
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("dial error: " + err.Error()))
		return 2
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString("write error: " + err.Error()))
		return 2
	}

	L.Push(lua.LBool(true))
	L.Push(lua.LNil)
	return 2
}

// Helper functions
func (e *Executor) toLuaValue(L *lua.LState, value interface{}) lua.LValue {
	if value == nil {
		return lua.LNil
	}

	switch v := value.(type) {
	case bool:
		return lua.LBool(v)
	case int:
		return lua.LNumber(v)
	case int8:
		return lua.LNumber(v)
	case int16:
		return lua.LNumber(v)
	case int32:
		return lua.LNumber(v)
	case int64:
		return lua.LNumber(v)
	case uint:
		return lua.LNumber(v)
	case uint8:
		return lua.LNumber(v)
	case uint16:
		return lua.LNumber(v)
	case uint32:
		return lua.LNumber(v)
	case uint64:
		return lua.LNumber(v)
	case float32:
		return lua.LNumber(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case []byte:
		// Convert binary data (like JPEG snapshots) to Lua string directly
		// In Lua, strings can hold binary data
		return lua.LString(string(v))
	case map[string]interface{}:
		table := L.NewTable()
		for k, val := range v {
			table.RawSetString(k, e.toLuaValue(L, val))
		}
		return table
	case []interface{}:
		table := L.NewTable()
		for i, val := range v {
			table.RawSetInt(i+1, e.toLuaValue(L, val))
		}
		return table
	default:
		// Fallback: try to convert to string
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

func (e *Executor) fromLuaValue(value lua.LValue) interface{} {
	switch v := value.(type) {
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		// Try to detect if it's an array or object
		maxN := 0
		v.ForEach(func(key, val lua.LValue) {
			if num, ok := key.(lua.LNumber); ok {
				if int(num) > maxN {
					maxN = int(num)
				}
			}
		})

		if maxN > 0 {
			// It's an array
			arr := make([]interface{}, maxN)
			v.ForEach(func(key, val lua.LValue) {
				if num, ok := key.(lua.LNumber); ok {
					arr[int(num)-1] = e.fromLuaValue(val)
				}
			})
			return arr
		} else {
			// It's an object
			obj := make(map[string]interface{})
			v.ForEach(func(key, val lua.LValue) {
				if str, ok := key.(lua.LString); ok {
					obj[string(str)] = e.fromLuaValue(val)
				}
			})
			return obj
		}
	default:
		return value.String()
	}
}

// Timer functions

// timerAfter schedules a timer to run after specified duration
// Usage: timer.after(60, callback) or timer.after(60, "timer_id", callback)
func (e *Executor) timerAfter(L *lua.LState) int {
	if e.scheduler == nil {
		logger.Warn("[LUA] Scheduler not available for timer.after")
		L.Push(lua.LNil)
		return 1
	}

	seconds := L.CheckNumber(1)

	var timerID string
	var callback *lua.LFunction

	// Check if second arg is string (custom ID) or function (callback)
	if L.GetTop() >= 3 {
		// timer.after(seconds, "id", callback)
		timerID = L.CheckString(2)
		callback = L.CheckFunction(3)
	} else {
		// timer.after(seconds, callback) - auto-generate ID
		callback = L.CheckFunction(2)
		timerID = fmt.Sprintf("timer_%d", time.Now().UnixNano())
	}

	// Type assertion to get scheduler methods
	type schedulerInterface interface {
		AddTimerCallback(id string, triggerTime time.Time, callback *lua.LFunction, state *lua.LState)
	}

	if sched, ok := e.scheduler.(schedulerInterface); ok {
		triggerTime := time.Now().Add(time.Duration(seconds) * time.Second)

		// Increment timer counter for this state
		timerCount := L.GetGlobal("__timer_count__")
		count := 0
		if timerCount != lua.LNil {
			if num, ok := timerCount.(lua.LNumber); ok {
				count = int(num)
			}
		}
		count++
		L.SetGlobal("__timer_count__", lua.LNumber(count))
		L.SetGlobal("__timers_created__", lua.LTrue)

		logger.Debug("Lua state %p now has %d active timer(s)", L, count)

		sched.AddTimerCallback(timerID, triggerTime, callback, L)
		L.Push(lua.LString(timerID))
	} else {
		logger.Error("[LUA] Scheduler type assertion failed")
		L.Push(lua.LNil)
	}

	return 1
}

// timerAt schedules a timer at specific time (HH:MM format)
// Usage: timer.at("17:30", callback) or timer.at("17:30", "timer_id", callback)
func (e *Executor) timerAt(L *lua.LState) int {
	if e.scheduler == nil {
		logger.Warn("[LUA] Scheduler not available for timer.at")
		L.Push(lua.LNil)
		return 1
	}

	timeStr := L.CheckString(1)

	var timerID string
	var callback *lua.LFunction

	// Check if second arg is string (custom ID) or function (callback)
	if L.GetTop() >= 3 {
		// timer.at("17:30", "id", callback)
		timerID = L.CheckString(2)
		callback = L.CheckFunction(3)
	} else {
		// timer.at("17:30", callback) - auto-generate ID
		callback = L.CheckFunction(2)
		timerID = fmt.Sprintf("timer_%d", time.Now().UnixNano())
	}

	// Parse HH:MM format
	var hour, minute int
	if _, err := fmt.Sscanf(timeStr, "%d:%d", &hour, &minute); err != nil {
		logger.Error("[LUA] Invalid time format: %s (expected HH:MM)", timeStr)
		L.Push(lua.LNil)
		return 1
	}

	now := time.Now()
	triggerTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// If time has passed today, schedule for tomorrow
	if triggerTime.Before(now) {
		triggerTime = triggerTime.Add(24 * time.Hour)
	}

	type schedulerInterface interface {
		AddTimerCallback(id string, triggerTime time.Time, callback *lua.LFunction, state *lua.LState)
	}

	if sched, ok := e.scheduler.(schedulerInterface); ok {
		// Increment timer counter
		timerCount := L.GetGlobal("__timer_count__")
		count := 0
		if timerCount != lua.LNil {
			if num, ok := timerCount.(lua.LNumber); ok {
				count = int(num)
			}
		}
		count++
		L.SetGlobal("__timer_count__", lua.LNumber(count))
		L.SetGlobal("__timers_created__", lua.LTrue)

		logger.Debug("Lua state %p now has %d active timer(s)", L, count)

		sched.AddTimerCallback(timerID, triggerTime, callback, L)
		L.Push(lua.LString(timerID))
	} else {
		logger.Error("[LUA] Scheduler type assertion failed")
		L.Push(lua.LNil)
	}

	return 1
}

// timerEvery creates a recurring timer
// Usage: timer.every(300, callback) or timer.every(300, "timer_id", callback)
func (e *Executor) timerEvery(L *lua.LState) int {
	if e.scheduler == nil {
		logger.Warn("[LUA] Scheduler not available for timer.every")
		L.Push(lua.LNil)
		return 1
	}

	seconds := L.CheckNumber(1)

	var timerID string
	var callback *lua.LFunction

	// Check if second arg is string (custom ID) or function (callback)
	if L.GetTop() >= 3 {
		// timer.every(300, "id", callback)
		timerID = L.CheckString(2)
		callback = L.CheckFunction(3)
	} else {
		// timer.every(300, callback) - auto-generate ID
		callback = L.CheckFunction(2)
		timerID = fmt.Sprintf("timer_%d", time.Now().UnixNano())
	}

	type schedulerInterface interface {
		AddRecurringTimerCallback(id string, interval time.Duration, callback *lua.LFunction, state *lua.LState)
	}

	if sched, ok := e.scheduler.(schedulerInterface); ok {
		interval := time.Duration(seconds) * time.Second

		// Increment timer counter
		timerCount := L.GetGlobal("__timer_count__")
		count := 0
		if timerCount != lua.LNil {
			if num, ok := timerCount.(lua.LNumber); ok {
				count = int(num)
			}
		}
		count++
		L.SetGlobal("__timer_count__", lua.LNumber(count))
		L.SetGlobal("__timers_created__", lua.LTrue)

		logger.Debug("Lua state %p now has %d active timer(s)", L, count)

		sched.AddRecurringTimerCallback(timerID, interval, callback, L)
		L.Push(lua.LString(timerID))
	} else {
		logger.Error("[LUA] Scheduler type assertion failed")
		L.Push(lua.LNil)
	}

	return 1
}

// timerCancel cancels a timer
// Usage: timer.cancel("timer_id")
func (e *Executor) timerCancel(L *lua.LState) int {
	if e.scheduler == nil {
		logger.Warn("[LUA] Scheduler not available for timer.cancel")
		L.Push(lua.LFalse)
		return 1
	}

	timerID := L.CheckString(1)

	type schedulerInterface interface {
		RemoveTimer(id string) bool
	}

	if sched, ok := e.scheduler.(schedulerInterface); ok {
		result := sched.RemoveTimer(timerID)
		L.Push(lua.LBool(result))
	} else {
		logger.Error("[LUA] Scheduler type assertion failed")
		L.Push(lua.LFalse)
	}

	return 1
}

// timerList returns list of active timers
// Usage: local timers = timer.list()
func (e *Executor) timerList(L *lua.LState) int {
	if e.scheduler == nil {
		logger.Warn("[LUA] Scheduler not available for timer.list")
		L.Push(L.NewTable())
		return 1
	}

	type schedulerInterface interface {
		ListTimers() []string
	}

	if sched, ok := e.scheduler.(schedulerInterface); ok {
		timers := sched.ListTimers()
		table := L.NewTable()
		for i, id := range timers {
			table.RawSetInt(i+1, lua.LString(id))
		}
		L.Push(table)
	} else {
		logger.Error("[LUA] Scheduler type assertion failed")
		L.Push(L.NewTable())
	}

	return 1
}
