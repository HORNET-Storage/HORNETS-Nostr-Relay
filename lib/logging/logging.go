package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/spf13/viper"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel converts a string to LogLevel
func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// Logger represents our custom logger
type Logger struct {
	level      LogLevel
	format     string
	output     string
	logDir     string
	currentLog *os.File
	mu         sync.RWMutex
	started    time.Time
}

var (
	globalLogger *Logger
	once         sync.Once
)

// InitLogger initializes the global logger with config
func InitLogger() error {
	var err error
	once.Do(func() {
		globalLogger, err = NewLogger()
	})
	return err
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		// Fallback to basic logger if not initialized
		globalLogger, _ = NewBasicLogger()
	}
	return globalLogger
}

// NewLogger creates a new logger instance using the global config
func NewLogger() (*Logger, error) {
	logger := &Logger{
		level:   ParseLogLevel(viper.GetString("logging.level")),
		output:  viper.GetString("logging.output"),
		logDir:  config.GetPath("logs"),
		started: time.Now(),
	}

	if err := logger.setupOutput(); err != nil {
		return nil, fmt.Errorf("failed to setup logger output: %w", err)
	}

	return logger, nil
}

// NewBasicLogger creates a basic logger for fallback
func NewBasicLogger() (*Logger, error) {
	return &Logger{
		level:   INFO,
		format:  "text",
		output:  "stdout",
		started: time.Now(),
	}, nil
}

// setupOutput configures the output destination
func (l *Logger) setupOutput() error {
	if l.output == "stdout" {
		return nil
	}

	if l.output == "file" || l.output == "both" {
		if err := l.createLogFile(); err != nil {
			return err
		}
	}

	return nil
}

// createLogFile creates the log file with date/time structure
func (l *Logger) createLogFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Create directory structure: logs/2025-06-17/14-30-45.log
	now := l.started
	dateDir := now.Format("2006-01-02")
	timeFile := now.Format("15-04-05") + ".log"

	fullDir := filepath.Join(l.logDir, dateDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(fullDir, timeFile)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Close previous file if exists
	if l.currentLog != nil {
		l.currentLog.Close()
	}

	l.currentLog = file
	return nil
}

// shouldLog determines if a message should be logged based on level
func (l *Logger) shouldLog(level LogLevel) bool {
	return level >= l.level
}

// getWriter returns the appropriate writer(s)
func (l *Logger) getWriter() io.Writer {
	l.mu.RLock()
	defer l.mu.RUnlock()

	switch l.output {
	case "stdout":
		return os.Stdout
	case "file":
		if l.currentLog != nil {
			return l.currentLog
		}
		return os.Stdout
	case "both":
		if l.currentLog != nil {
			return io.MultiWriter(os.Stdout, l.currentLog)
		}
		return os.Stdout
	default:
		return os.Stdout
	}
}

// formatMessage formats the log message based on the configured format
func (l *Logger) formatMessage(level LogLevel, msg string, fields map[string]interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	return l.formatText(timestamp, level, msg, fields)
}

// formatText formats message as plain text
func (l *Logger) formatText(timestamp string, level LogLevel, msg string, fields map[string]interface{}) string {
	result := fmt.Sprintf("%s [%s] %s", timestamp, level.String(), msg)

	if len(fields) > 0 {
		result += " |"
		for k, v := range fields {
			result += fmt.Sprintf(" %s=%v", k, v)
		}
	}

	return result
}

// log is the core logging method
func (l *Logger) log(level LogLevel, msg string, fields map[string]interface{}) {
	if !l.shouldLog(level) {
		return
	}

	formatted := l.formatMessage(level, msg, fields)
	writer := l.getWriter()

	fmt.Fprintln(writer, formatted)

	// For FATAL logs, exit the application
	if level == FATAL {
		os.Exit(1)
	}
}

// Public logging methods

// Debug logs debug level messages
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(DEBUG, msg, f)
}

// Info logs info level messages
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(INFO, msg, f)
}

// Warn logs warning level messages
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(WARN, msg, f)
}

// Error logs error level messages
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ERROR, msg, f)
}

// Fatal logs fatal level messages and exits
func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(FATAL, msg, f)
}

// Formatted logging methods with printf-style formatting

// Debugf logs debug level messages with formatting
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Infof logs info level messages with formatting
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs warning level messages with formatting
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs error level messages with formatting
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// Fatalf logs fatal level messages with formatting and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Fatal(fmt.Sprintf(format, args...))
}

// Close closes the logger and any open files
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentLog != nil {
		return l.currentLog.Close()
	}
	return nil
}

// Global convenience functions

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...map[string]interface{}) {
	GetLogger().Debug(msg, fields...)
}

// Info logs an info message using the global logger
func Info(msg string, fields ...map[string]interface{}) {
	GetLogger().Info(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...map[string]interface{}) {
	GetLogger().Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, fields ...map[string]interface{}) {
	GetLogger().Error(msg, fields...)
}

// Fatal logs a fatal message using the global logger and exits
func Fatal(msg string, fields ...map[string]interface{}) {
	GetLogger().Fatal(msg, fields...)
}

// Formatted global functions

// Debugf logs a formatted debug message using the global logger
func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

// Infof logs a formatted info message using the global logger
func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

// Warnf logs a formatted warning message using the global logger
func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

// Errorf logs a formatted error message using the global logger
func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

// Fatalf logs a formatted fatal message using the global logger and exits
func Fatalf(format string, args ...interface{}) {
	GetLogger().Fatalf(format, args...)
}
