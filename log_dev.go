// +build dev

package opbot

import (
	log "github.com/sirupsen/logrus"
)

func devdbg(format string, args ...interface{}) {
	log.Debugf(format, args...)
}
