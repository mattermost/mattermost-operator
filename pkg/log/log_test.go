package log

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var logrus = log.NewEntry(log.New())
var defaultLogger = &logrusLogger{
	l: logrus,
	infoLogger: infoLogger{
		l:    logrus,
		name: "default",
	},
}

func TestNewLogger(t *testing.T) {
	newLogger := newLogger(defaultLogger)
	assert.Equal(t, newLogger, defaultLogger)

	// Change the newLogger and make sure the original isn't modified.
	newLogger.name = "newName"
	assert.NotEqual(t, newLogger, defaultLogger)
}

func TestEnabled(t *testing.T) {
	logger := InitLogger()
	assert.True(t, logger.Enabled())
}

func TestLevel(t *testing.T) {
	levelTests := []int{-1000, -1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 100, 1000}
	for _, level := range levelTests {
		t.Run(fmt.Sprintf("level-%d", level), func(t *testing.T) {
			logger := InitLogger()
			assert.NotPanics(t, func() { logger.V(level) })
		})
	}
}

func TestParseFields(t *testing.T) {
	var parseTests = []struct {
		name     string
		args     []string
		expected log.Fields
	}{
		{
			"noArgs",
			[]string{},
			make(map[string]interface{}),
		}, {
			"oneArgs",
			[]string{"key1"},
			make(map[string]interface{}),
		}, {
			"twoArgs",
			[]string{"key1", "val1"},
			map[string]interface{}{
				"key1": "val1",
			},
		}, {
			"threeArgs",
			[]string{"key1", "val1", "key2"},
			map[string]interface{}{
				"key1": "val1",
			},
		}, {
			"fourArgs",
			[]string{"key1", "val1", "key2", "val2"},
			map[string]interface{}{
				"key1": "val1",
				"key2": "val2",
			},
		},
	}
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			var args []interface{}
			for _, arg := range tt.args {
				args = append(args, arg)
			}
			assert.Equal(
				t,
				parseFields(defaultLogger.l, tt.name, args),
				tt.expected,
			)
		})
	}
}
