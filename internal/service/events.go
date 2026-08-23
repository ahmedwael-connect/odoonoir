package service

import (
	"github.com/ahmed/odoonoir/internal/updater"
)

// EventKind discriminates service events.
type EventKind int

const (
	// StatusChanged fires when an instance transitions running/stopped.
	StatusChanged EventKind = iota
	// StepStart marks a long-running pipeline step beginning.
	StepStart
	// StepDone marks a pipeline step completing successfully.
	StepDone
	// StepFail marks a pipeline step failing (Message holds the error).
	StepFail
	// LogLine streams one line of command/log output.
	LogLine
)

// Event is emitted by service operations. UI layers render it without
// knowing anything about the underlying process orchestration.
type Event struct {
	Kind     EventKind `json:"kind"`
	Instance string    `json:"instance,omitempty"`
	Step     string    `json:"step,omitempty"`
	Message  string    `json:"message,omitempty"`
}

// Sink receives events. Sinks must be fast (no blocking I/O) — heavy work
// belongs on the receiver side.
type Sink func(Event)

// NopSink discards events; useful for tests and headless callers.
func NopSink(Event) {}

// UpdaterSink adapts updater.Progress events into service Events, so the
// update/init pipelines stream through the same event channel as everything
// else.
func UpdaterSink(instName string, sink Sink) func(updater.Progress) {
	return func(p updater.Progress) {
		e := Event{Instance: instName, Step: p.Name}
		switch p.Kind {
		case updater.StepStart:
			e.Kind = StepStart
		case updater.StepDone:
			e.Kind = StepDone
		default:
			e.Kind = LogLine
			e.Message = p.Line
		}
		sink(e)
	}
}

// LineSink adapts a plain line callback (used by InitDatabase,
// InstallModules, Restore...) into service Events.
func LineSink(instName, step string, sink Sink) func(string) {
	return func(line string) { sink(Event{Kind: LogLine, Instance: instName, Step: step, Message: line}) }
}
