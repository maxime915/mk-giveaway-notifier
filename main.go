package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
)

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	runtime.SetBlockProfileRate(1)

	if useChannel {
		channel()
	} else {
		useStream()
	}
}

