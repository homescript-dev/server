package scheduler

import (
	"fmt"
	"homescript-server/internal/events"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
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
		lastDay:    now.Day(), // Initialize with current day to prevent duplicate calculation
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

	logger.Debug("Scheduler ticker started, checking time events every second")

	for {
		select {
		case <-s.stopChan:
			logger.Debug("Scheduler received stop signal")
			return
		case now := <-ticker.C:
			s.checkTimeEvents(now)
		}
	}
}

func (s *Scheduler) checkTimeEvents(now time.Time) {
	now = now.In(s.location)

	minute := now.Minute()
	hour := now.Hour()
	second := now.Second()
	day := now.Day()
	weekday := int(now.Weekday())

	// Update sunrise/sunset times if day changed
	if day != s.lastDay {
		s.lastDay = day
		s.updateSunTimes(now)
	}

	// Only trigger minute-based events when second = 0 (start of minute)
	// This prevents triggering on startup if server starts mid-minute
	if second != 0 {
		return
	}

	// Every minute event
	if minute != s.lastMinute {
		s.lastMinute = minute
		s.triggerEvent("every_minute", now, weekday)

		// Also trigger custom time events in format HH_MM (e.g., 17_05 for 17:05)
		customTime := fmt.Sprintf("%02d_%02d", hour, minute)
		s.triggerEvent(customTime, now, weekday)

		// Check for sunrise (within the same minute)
		if !s.sunriseTime.IsZero() &&
			hour == s.sunriseTime.Hour() &&
			minute == s.sunriseTime.Minute() {
			logger.Info("Sunrise at %02d:%02d", hour, minute)
			s.triggerEvent("sunrise", now, weekday)
		}

		// Check for sunset (within the same minute)
		if !s.sunsetTime.IsZero() &&
			hour == s.sunsetTime.Hour() &&
			minute == s.sunsetTime.Minute() {
			logger.Info("Sunset at %02d:%02d", hour, minute)
			s.triggerEvent("sunset", now, weekday)
		}
	}

	// Every hour event (at XX:00)
	if hour != s.lastHour && minute == 0 {
		s.lastHour = hour
		s.triggerEvent("every_hour", now, weekday)
	}

	// Check dynamic timers
	s.checkTimers(now)
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
		// No coordinates configured, use fixed times as fallback
		logger.Warn("No latitude/longitude configured, using fixed sunrise/sunset times (06:30/18:30)")
		s.sunriseTime = time.Date(now.Year(), now.Month(), now.Day(), 6, 30, 0, 0, s.location)
		s.sunsetTime = time.Date(now.Year(), now.Month(), now.Day(), 18, 30, 0, 0, s.location)
		return
	}

	// Calculate actual sunrise and sunset
	sunrise, sunset := sunrise.SunriseSunset(
		s.latitude, s.longitude,
		now.Year(), now.Month(), now.Day(),
	)

	s.sunriseTime = sunrise.In(s.location)
	s.sunsetTime = sunset.In(s.location)

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
