package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// Level represents logging level
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	CRITICAL
)

var levelNames = map[Level]string{
	DEBUG:    "DEBUG",
	INFO:     "INFO",
	WARN:     "WARN",
	ERROR:    "ERROR",
	CRITICAL: "CRITICAL",
}

var levelColors = map[Level]string{
	DEBUG:    "\033[36m", // Cyan
	INFO:     "\033[32m", // Green
	WARN:     "\033[33m", // Yellow
	ERROR:    "\033[31m", // Red
	CRITICAL: "\033[35m", // Magenta
}

const colorReset = "\033[0m"

// Logger provides leveled logging
type Logger struct {
	level       Level
	output      io.Writer
	useColors   bool
	debugLog    *log.Logger
	infoLog     *log.Logger
	warnLog     *log.Logger
	errorLog    *log.Logger
	criticalLog *log.Logger
}

var defaultLogger *Logger

// Init initializes the default logger
func Init(level Level, useColors bool) {
	defaultLogger = New(level, os.Stdout, useColors)
}

// New creates a new Logger
func New(level Level, output io.Writer, useColors bool) *Logger {
	flags := log.Ldate | log.Ltime

	return &Logger{
		level:       level,
		output:      output,
		useColors:   useColors,
		debugLog:    log.New(output, "", flags),
		infoLog:     log.New(output, "", flags),
		warnLog:     log.New(output, "", flags),
		errorLog:    log.New(output, "", flags),
		criticalLog: log.New(output, "", flags),
	}
}

// SetLevel changes the logging level
func SetLevel(level Level) {
	if defaultLogger != nil {
		defaultLogger.level = level
	}
}

// GetLevel returns current logging level
func GetLevel() Level {
	if defaultLogger != nil {
		return defaultLogger.level
	}
	return ERROR
}

// ParseLevel parses level string to Level
func ParseLevel(s string) (Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARN", "WARNING":
		return WARN, nil
	case "ERROR":
		return ERROR, nil
	case "CRITICAL":
		return CRITICAL, nil
	default:
		return ERROR, fmt.Errorf("invalid log level: %s", s)
	}
}

func (l *Logger) log(level Level, format string, v ...interface{}) {
	if l == nil || level < l.level {
		return
	}

	var logInstance *log.Logger
	switch level {
	case DEBUG:
		logInstance = l.debugLog
	case INFO:
		logInstance = l.infoLog
	case WARN:
		logInstance = l.warnLog
	case ERROR:
		logInstance = l.errorLog
	case CRITICAL:
		logInstance = l.criticalLog
	}

	if logInstance == nil {
		return
	}

	levelName := levelNames[level]
	prefix := fmt.Sprintf("[%s] ", levelName)

	if l.useColors {
		color := levelColors[level]
		prefix = color + prefix + colorReset
	}

	message := fmt.Sprintf(format, v...)
	logInstance.Print(prefix + message)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	l.log(DEBUG, format, v...)
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	l.log(INFO, format, v...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, v ...interface{}) {
	l.log(WARN, format, v...)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	l.log(ERROR, format, v...)
}

// Critical logs a critical message
func (l *Logger) Critical(format string, v ...interface{}) {
	l.log(CRITICAL, format, v...)
}

// Global functions using default logger

// Debug logs a debug message using default logger
func Debug(format string, v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, v...)
	}
}

// Info logs an info message using default logger
func Info(format string, v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, v...)
	}
}

// Warn logs a warning message using default logger
func Warn(format string, v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(format, v...)
	}
}

// Error logs an error message using default logger
func Error(format string, v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, v...)
	}
}

// Critical logs a critical message using default logger
func Critical(format string, v ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Critical(format, v...)
	}
}
