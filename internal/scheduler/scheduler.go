package scheduler

import (
	"fmt"
	"homescript-server/internal/events"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nathan-osman/go-sunrise"
)

// Scheduler handles time-based events
type Scheduler struct {
	router      *events.Router
	executor    interface{} // Executor interface to avoid circular dependency
	stopChan    chan struct{}
	wg          sync.WaitGroup
	location    *time.Location
	latitude    float64
	longitude   float64
	lastMinute  int
	lastHour    int
	lastDay     int
	sunriseTime time.Time
	sunsetTime  time.Time
	timers      map[string]*Timer
	timersMutex sync.RWMutex
}

// Timer represents a one-time or recurring scheduled event
type Timer struct {
	ID          string
	TriggerTime time.Time
	Callback    []byte // Lua bytecode
	Recurring   bool
	Interval    time.Duration
}

// Config holds scheduler configuration
type Config struct {
	// Location for time calculations (optional, defaults to Local)
	Location *time.Location
	// Latitude for sunrise/sunset calculations (required for accurate times)
	Latitude float64
	// Longitude for sunrise/sunset calculations (required for accurate times)
	Longitude float64
}

// New creates a new scheduler
func New(router *events.Router, cfg Config) *Scheduler {
	location := cfg.Location
	if location == nil {
		location = time.Local
	}

	now := time.Now()

	s := &Scheduler{
		router:     router,
		stopChan:   make(chan struct{}),
		location:   location,
		latitude:   cfg.Latitude,
		longitude:  cfg.Longitude,
		lastMinute: -1,
		lastHour:   -1,
		lastDay:    now.Day(),
		timers:     make(map[string]*Timer),
	}

	// Calculate sunrise/sunset for today
	s.updateSunTimes(now)

	return s
}

// SetExecutor sets the executor reference for callback execution
func (s *Scheduler) SetExecutor(exec interface{}) {
	s.executor = exec
}

// Start begins the scheduler
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
	logger.Info("Scheduler started")
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	logger.Info("Scheduler stopped")
}

func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	logger.Debug("Scheduler ticker started, checking events every second")

	for {
		select {
		case <-s.stopChan:
			logger.Debug("Scheduler received stop signal")
			return
		case now := <-ticker.C:
			// Check timers every second (they need 1-second precision)
			s.checkTimers(now)

			// Check time events only at second 0 (minute precision)
			if now.Second() == 0 {
				s.checkTimeEvents(now)
			}
		}
	}
}

func (s *Scheduler) checkTimeEvents(now time.Time) {
	minute := now.Minute()
	hour := now.Hour()
	day := now.Day()
	weekday := int(now.Weekday())

	// Update sunrise/sunset times if day changed
	if day != s.lastDay {
		s.lastDay = day
		s.updateSunTimes(now)
	}

	if minute != s.lastMinute {
		s.lastMinute = minute

		// Check and trigger wildcard: *_* (every minute)
		s.checkAndTrigger("*_*", now, weekday)

		// Check and trigger wildcard: *_XX (every hour at specific minute)
		s.checkAndTrigger(fmt.Sprintf("*_%02d", minute), now, weekday)

		// Check and trigger wildcard: HH_* (every minute of specific hour)
		s.checkAndTrigger(fmt.Sprintf("%02d_*", hour), now, weekday)

		// Check and trigger custom time: HH_MM
		s.checkAndTrigger(fmt.Sprintf("%02d_%02d", hour, minute), now, weekday)

		// Check and trigger sunrise
		if !s.sunriseTime.IsZero() && hour == s.sunriseTime.Hour() && minute == s.sunriseTime.Minute() {
			if s.checkAndTrigger("sunrise", now, weekday) {
				logger.Info("Sunrise at %02d:%02d", hour, minute)
			}
		}

		// Check and trigger sunset
		if !s.sunsetTime.IsZero() && hour == s.sunsetTime.Hour() && minute == s.sunsetTime.Minute() {
			if s.checkAndTrigger("sunset", now, weekday) {
				logger.Info("Sunset at %02d:%02d", hour, minute)
			}
		}

		// Check sunrise/sunset offsets
		s.checkSunOffsetEvents(now, hour, minute, weekday)
	}

	if hour != s.lastHour && minute == 0 {
		s.lastHour = hour
		s.checkAndTrigger("every_hour", now, weekday)
	}
}

// checkSunOffsetEvents checks for sunrise/sunset offset events
func (s *Scheduler) checkSunOffsetEvents(now time.Time, hour, minute, weekday int) {
	if s.sunriseTime.IsZero() || s.sunsetTime.IsZero() {
		return
	}

	sunrisePath := filepath.Join(s.router.GetBasePath(), "events", "time", "sunrise")
	s.checkOffsetDirectory(sunrisePath, s.sunriseTime, now, hour, minute, weekday)

	sunsetPath := filepath.Join(s.router.GetBasePath(), "events", "time", "sunset")
	s.checkOffsetDirectory(sunsetPath, s.sunsetTime, now, hour, minute, weekday)
}

// checkOffsetDirectory checks for offset time directories under sunrise/sunset
func (s *Scheduler) checkOffsetDirectory(basePath string, baseTime, now time.Time, currentHour, currentMinute, weekday int) {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Parse offset format: -00_30 or +01_30
		if len(name) < 6 || (name[0] != '-' && name[0] != '+') {
			continue
		}

		sign := name[0]
		timeStr := name[1:]

		var offsetHour, offsetMinute int
		if _, err := fmt.Sscanf(timeStr, "%02d_%02d", &offsetHour, &offsetMinute); err != nil {
			continue
		}

		// Calculate offset in minutes
		offsetMinutes := offsetHour*60 + offsetMinute
		if sign == '-' {
			offsetMinutes = -offsetMinutes
		}

		// Calculate target time
		targetTime := baseTime.Add(time.Duration(offsetMinutes) * time.Minute)

		// Check if current time matches
		if targetTime.Hour() == currentHour && targetTime.Minute() == currentMinute {
			eventPath := fmt.Sprintf("sunrise/%s", name)
			if filepath.Base(basePath) == "sunset" {
				eventPath = fmt.Sprintf("sunset/%s", name)
			}

			// Check if handler script exists
			scriptPath := filepath.Join(basePath, name, "handler.lua")
			if _, err := os.Stat(scriptPath); err == nil {
				s.triggerEvent(eventPath, now, weekday)
				logger.Info("Triggered offset event: %s at %02d:%02d", eventPath, currentHour, currentMinute)
			}
		}
	}
}

// checkTimers checks and triggers any due timers
func (s *Scheduler) checkTimers(now time.Time) {
	s.timersMutex.Lock()
	defer s.timersMutex.Unlock()

	for id, timer := range s.timers {
		if now.After(timer.TriggerTime) || now.Equal(timer.TriggerTime) {
			logger.Debug("Triggering timer: %s", id)

			// Execute the callback via executor
			if timer.Callback != nil && s.executor != nil {
				go s.executeTimerCallback(timer)
			}

			// Handle recurring timers
			if timer.Recurring {
				timer.TriggerTime = now.Add(timer.Interval)
				logger.Debug("Timer %s rescheduled for %s", id, timer.TriggerTime.Format("15:04:05"))
			} else {
				// Remove one-time timer
				delete(s.timers, id)
				logger.Debug("Timer %s removed (one-time)", id)
			}
		}
	}
}

// executeTimerCallback executes a timer callback in a goroutine
func (s *Scheduler) executeTimerCallback(timer *Timer) {
	if executor, ok := s.executor.(interface {
		ExecuteCallback(bytecode []byte, timerID string) error
	}); ok {
		if err := executor.ExecuteCallback(timer.Callback, timer.ID); err != nil {
			logger.Error("Timer %s callback failed: %v", timer.ID, err)
		}
	} else {
		logger.Error("Executor does not support ExecuteCallback")
	}
}

// AddTimerCallback adds a new callback-based timer to the scheduler
func (s *Scheduler) AddTimerCallback(id string, triggerTime time.Time, bytecode []byte) {
	s.timersMutex.Lock()
	defer s.timersMutex.Unlock()

	s.timers[id] = &Timer{
		ID:          id,
		TriggerTime: triggerTime,
		Callback:    bytecode,
		Recurring:   false,
	}

	logger.Info("Timer added: %s at %s", id, triggerTime.Format("2006-01-02 15:04:05"))
}

// AddRecurringTimerCallback adds a recurring callback-based timer
func (s *Scheduler) AddRecurringTimerCallback(id string, interval time.Duration, bytecode []byte) {
	s.timersMutex.Lock()
	defer s.timersMutex.Unlock()

	triggerTime := time.Now().Add(interval)

	s.timers[id] = &Timer{
		ID:          id,
		TriggerTime: triggerTime,
		Callback:    bytecode,
		Recurring:   true,
		Interval:    interval,
	}

	logger.Info("Recurring timer added: %s every %s", id, interval.String())
}

// RemoveTimer removes a timer by ID
func (s *Scheduler) RemoveTimer(id string) bool {
	s.timersMutex.Lock()
	defer s.timersMutex.Unlock()

	if _, exists := s.timers[id]; exists {
		delete(s.timers, id)
		logger.Info("Timer removed: %s", id)
		return true
	}
	return false
}

// ListTimers returns all active timer IDs
func (s *Scheduler) ListTimers() []string {
	s.timersMutex.RLock()
	defer s.timersMutex.RUnlock()

	ids := make([]string, 0, len(s.timers))
	for id := range s.timers {
		ids = append(ids, id)
	}
	return ids
}

// updateSunTimes calculates sunrise and sunset times for the given day
func (s *Scheduler) updateSunTimes(now time.Time) {
	if s.latitude == 0 && s.longitude == 0 {
		logger.Error("Cannot calculate sunrise/sunset: no coordinates available")
		s.sunriseTime = time.Time{}
		s.sunsetTime = time.Time{}
		return
	}

	sunrise, sunset := sunrise.SunriseSunset(
		s.latitude, s.longitude,
		now.Year(), now.Month(), now.Day(),
	)

	s.sunriseTime = sunrise
	s.sunsetTime = sunset

	logger.Info("Calculated sun times for %s: sunrise %02d:%02d, sunset %02d:%02d",
		now.Format("2006-01-02"),
		s.sunriseTime.Hour(), s.sunriseTime.Minute(),
		s.sunsetTime.Hour(), s.sunsetTime.Minute())
}

func (s *Scheduler) triggerEvent(eventType string, now time.Time, weekday int) {
	if s.router == nil {
		logger.Error("Scheduler router is nil, cannot trigger event: %s", eventType)
		return
	}

	event := &types.Event{
		Source: "time",
		Type:   eventType,
		Data: map[string]interface{}{
			"time":    now.Unix(),
			"hour":    now.Hour(),
			"minute":  now.Minute(),
			"second":  now.Second(),
			"weekday": weekday,
		},
		Timestamp: now,
	}

	logger.Debug("Triggering time event: %s at %02d:%02d:%02d", eventType, now.Hour(), now.Minute(), now.Second())
	s.router.RouteEvent(event)
}

// checkAndTrigger checks if script exists and triggers event if it does
func (s *Scheduler) checkAndTrigger(eventType string, now time.Time, weekday int) bool {
	timeBasePath := filepath.Join(s.router.GetBasePath(), "events", "time")
	scriptPath := filepath.Join(timeBasePath, eventType, "handler.lua")

	if _, err := os.Stat(scriptPath); err == nil {
		s.triggerEvent(eventType, now, weekday)
		return true
	}
	return false
}
