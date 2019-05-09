package log

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
)

type logrusLogger struct {
	l *log.Entry
	infoLogger
}

type infoLogger struct {
	l    *log.Entry
	name string
}

// InitLogger returns a logrus-based logger wrapped in a logr.Logger interface.
func InitLogger() logr.Logger {
	logger := log.NewEntry(log.New())
	return &logrusLogger{
		l: logger,
		infoLogger: infoLogger{
			l:    logger,
			name: "",
		},
	}
}

func (l *infoLogger) Enabled() bool { return true }
func (l *infoLogger) Info(msg string, keysAndVals ...interface{}) {
	fields := parseFields(l.l, l.name, keysAndVals)
	l.l.WithFields(fields).Info(prependName(l.name, msg))
}

func (l *logrusLogger) Error(err error, msg string, keysAndVals ...interface{}) {
	fields := parseFields(l.l, l.name, keysAndVals)
	fields["err.ctx"] = msg
	l.l.WithFields(fields).Error(prependName(l.name, err.Error()))
}

// TODO
// The commented code below works, but in practice the operator logging quickly
// gets set to 'fatal' and stops logging normally. This requires further
// investigation. For now, the logging levels are not changed.
func (l *logrusLogger) V(level int) logr.InfoLogger {
	// Compare the given int to the list of all logrus levels. If the given value
	// is below or above the range of logrus levels then we take the lowest or
	// highest value respectively.
	// logLevel := log.AllLevels[0]
	// if level > 0 && level < (len(log.AllLevels)-1) {
	// 	logLevel = log.AllLevels[level]
	// } else if level > (len(log.AllLevels) - 1) {
	// 	logLevel = log.AllLevels[(len(log.AllLevels) - 1)]
	// }

	// newLogger := newLogger(l)
	// newLogger.l.Logger.SetLevel(logLevel)
	// return &infoLogger{
	// 	l:    newLogger.l,
	// 	name: l.name,
	// }

	return &infoLogger{
		l:    l.l,
		name: l.name,
	}
}

func (l *logrusLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	newFieldLogger := l.l.WithFields(parseFields(l.l, l.name, keysAndValues))
	newLogger := newLogger(l)
	newLogger.l = newFieldLogger
	newLogger.infoLogger.l = newFieldLogger

	return newLogger
}

func (l *logrusLogger) WithName(name string) logr.Logger {
	newLogger := newLogger(l)
	if l.name == "" {
		newLogger.name = name
	} else {
		newLogger.name = fmt.Sprintf("%s.%s", l.name, name)
	}

	return newLogger
}

func newLogger(l *logrusLogger) *logrusLogger {
	return &logrusLogger{
		l: l.l,
		infoLogger: infoLogger{
			l:    l.l,
			name: l.name,
		},
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
