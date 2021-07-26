package midiserver

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/geomyidia/erl-midi-server/pkg/port"
)

// ProcessCommand ...
func ProcessCommand(ctx context.Context, command string) {
	switch command {
	case "ping":
		port.SendResult("pong")
	case "example":
		Example()
		port.SendResult("ok")
	case "stop":
		log.Info("Stopping Go MIDI server ...")
		<-ctx.Done()
	default:
		port.SendError("Received unsupported command")
	}
}