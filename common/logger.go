package common

import (
	"log"
)

var logger *log.Logger

func GetLogger() *log.Logger {
	return logger
}
