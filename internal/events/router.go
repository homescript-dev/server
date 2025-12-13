package events

import (
	"homescript-server/internal/executor"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"os"
	"path/filepath"
	"strings"
)

// Router routes events to appropriate Lua scripts
type Router struct {
	basePath string
	pool     *executor.Pool
}

// New creates a new event router
func New(basePath string, pool *executor.Pool) *Router {
	return &Router{
		basePath: basePath,
		pool:     pool,
	}
}

// RouteEvent finds and executes scripts for the given event
func (r *Router) RouteEvent(event *types.Event) {
	scripts := r.findScripts(event)

	if len(scripts) == 0 {
		// More detailed debug info for device events
		if event.Source == "device" && event.Device != "" && event.Attribute != "" {
			logger.Debug("No scripts found for event: %s/%s (device: %s, attribute: %s)",
				event.Source, event.Type, event.Device, event.Attribute)
		} else {
			logger.Debug("No scripts found for event: %s/%s", event.Source, event.Type)
		}
		return
	}

	logger.Debug("Found %d script(s) for event: %s/%s", len(scripts), event.Source, event.Type)

	for _, scriptPath := range scripts {
		r.pool.Submit(executor.Task{
			ScriptPath: scriptPath,
			Event:      event,
		})
	}
}

func (r *Router) findScripts(event *types.Event) []string {
	var scripts []string

	switch event.Source {
	case "mqtt":
		scripts = append(scripts, r.findMQTTScripts(event)...)
	case "device":
		scripts = append(scripts, r.findDeviceScripts(event)...)
	case "time":
		scripts = append(scripts, r.findTimeScripts(event)...)
	case "state":
		scripts = append(scripts, r.findStateScripts(event)...)
	}

	return scripts
}

func (r *Router) findMQTTScripts(event *types.Event) []string {
	var scripts []string

	// Convert MQTT topic to file path
	// e.g., zigbee2mqtt/bedroom_light -> events/mqtt/zigbee2mqtt/bedroom_light
	topicPath := strings.ReplaceAll(event.Topic, "/", string(os.PathSeparator))
	basePath := filepath.Join(r.basePath, "events", "mqtt", topicPath)

	// Look for scripts in this directory
	scripts = append(scripts, r.findLuaFiles(basePath)...)

	return scripts
}

func (r *Router) findDeviceScripts(event *types.Event) []string {
	var scripts []string

	if event.Device == "" {
		return scripts
	}

	devicePath := filepath.Join(r.basePath, "events", "device", event.Device)

	// If there's a specific attribute, look in that directory
	if event.Attribute != "" {
		attrPath := filepath.Join(devicePath, event.Attribute)
		logger.Debug("Looking for device scripts in: %s", attrPath)
		scripts = append(scripts, r.findLuaFiles(attrPath)...)
		// Don't look in generic device directory to avoid duplicates
		return scripts
	}

	// Only look for generic device event handlers if no specific attribute
	logger.Debug("Looking for generic device scripts in: %s", devicePath)
	scripts = append(scripts, r.findLuaFiles(devicePath)...)

	return scripts
}

func (r *Router) findTimeScripts(event *types.Event) []string {
	var scripts []string

	timePath := filepath.Join(r.basePath, "events", "time", event.Type)
	scripts = append(scripts, r.findLuaFiles(timePath)...)

	return scripts
}

func (r *Router) findStateScripts(event *types.Event) []string {
	var scripts []string

	if event.Attribute == "" {
		return scripts
	}

	statePath := filepath.Join(r.basePath, "events", "state", event.Attribute)
	scripts = append(scripts, r.findLuaFiles(statePath)...)

	return scripts
}

func (r *Router) findLuaFiles(dir string) []string {
	var scripts []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return scripts
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if strings.HasSuffix(entry.Name(), ".lua") {
			fullPath := filepath.Join(dir, entry.Name())
			scripts = append(scripts, fullPath)
		}
	}

	return scripts
}
