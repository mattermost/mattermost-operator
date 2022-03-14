package log

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
)

type logrusLogger struct {
	logger       *log.Entry
	defaultLevel log.Level
	name         string
}

// InitLogger returns a logrus-based logger wrapped in a logr.LogSink interface.
func InitLogger(logger *log.Entry) logr.LogSink {

	return &logrusLogger{
		logger:       logger,
		defaultLevel: logger.Logger.Level,
		name:         "",
	}
}

func (l *logrusLogger) Init(info logr.RuntimeInfo) {
	// We initialize logger with constructor as we do not support CallDepth.
}

// Enabled tests whether logging is enabled on specified of higher defaultLevel.
func (l *logrusLogger) Enabled(level int) bool {
	return int(l.logger.Logger.Level) >= level
}

func (l *logrusLogger) Info(level int, msg string, keysAndVals ...interface{}) {
	fields := parseFields(l.logger, l.name, keysAndVals)
	l.logger.WithFields(fields).Log(l.toLogrusLevel(level), prependName(l.name, msg))
}

func (l *logrusLogger) Error(err error, msg string, keysAndVals ...interface{}) {
	fields := parseFields(l.logger, l.name, keysAndVals)
	l.logger.WithFields(fields).WithError(err).Error(prependName(l.name, msg))
}

func (l *logrusLogger) WithValues(keysAndValues ...interface{}) logr.LogSink {
	newFieldLogger := l.logger.WithFields(parseFields(l.logger, l.name, keysAndValues))
	newLogger := newLogger(l)
	newLogger.logger = newFieldLogger

	return newLogger
}

func (l *logrusLogger) WithName(name string) logr.LogSink {
	newLogger := newLogger(l)
	if l.name == "" {
		newLogger.name = name
	} else {
		newLogger.name = fmt.Sprintf("%s.%s", l.name, name)
	}

	return newLogger
}

func (l *logrusLogger) toLogrusLevel(level int) log.Level {
	lvl := log.Level(level)
	if lvl <= 0 {
		return l.defaultLevel
	}
	if lvl > log.TraceLevel {
		return log.TraceLevel
	}
	return lvl
}

func newLogger(l *logrusLogger) *logrusLogger {
	return &logrusLogger{
		logger:       l.logger,
		defaultLevel: l.defaultLevel,
		name:         l.name,
	}
}

func parseFields(le *log.Entry, name string, args []interface{}) log.Fields {
	fields := make(map[string]interface{})
	if len(args) == 0 {
		return fields
	}

	for i := 0; i < len(args); {
		// Make sure we don't have an extra key with no value.
		if i == len(args)-1 {
			le.Error(errors.New(
				prependName(name, "Logging key provided with no value; skipping"),
			), "KeyParseError")
			break
		}

		// Process a key value pair. If the key isn't a string then ignore and
		// skip remaining args.
		key, value := args[i], args[i+1]
		keyStr, isString := key.(string)
		if !isString {
			le.Error(errors.New(
				prependName(name,
					fmt.Sprintf("logging key %v is not a string; skipping remaining values", key),
				)), "KeyParseError")
			break
		}

		fields[keyStr] = value
		i += 2
	}

	return fields
}

func prependName(name, msg string) string {
	return fmt.Sprintf("[%s] %s", name, msg)
}
