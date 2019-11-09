package main

import (
	"log"
	"syscall"
)

func init() {
	increaseRlimitNofile()
}

func increaseRlimitNofile() {
	var lim syscall.Rlimit

	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		log.Printf("unable to get current RLIMIT_NOFILE: %v", err)
		return
	}
	lim.Cur = lim.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		log.Printf("unable to set new RLIMIT_NOFILE: %v", err)
		return
	}
}
