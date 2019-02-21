// +build dev

package opbot

import (
	log "github.com/Sirupsen/logrus"
)

func devdbg(format string, args ...interface{}) {
	log.Debugf(format, args...)
}
