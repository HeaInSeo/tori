package main

import (
	"github.com/HeaInSeo/tori/cmd"
	globallog "github.com/HeaInSeo/tori/log"
)

var logger = globallog.Log

func main() {
	if err := cmd.Execute(); err != nil {
		logger.Fatalf("명령 실행 실패: %v", err)
	}
}
