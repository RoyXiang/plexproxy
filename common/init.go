package common

import (
	"io"
	"log"
	"os"
)

func init() {
	log.SetOutput(io.Discard)
	logger = log.New(os.Stdout, "", log.LstdFlags)
}
