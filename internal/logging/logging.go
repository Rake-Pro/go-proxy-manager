// Package logging configures zerolog as the single global logger for the project.
// zerolog is the one accepted logging dependency (see CLAUDE.md).
package logging

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup initialises the global zerolog logger. level is a zerolog level string
// (trace/debug/info/warn/error); console enables human-friendly colored output
// for local development instead of JSON.
func Setup(level string, console bool) {
	zerolog.TimeFieldFormat = time.RFC3339

	lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil || level == "" {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	if console {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
}
