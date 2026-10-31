package main

import "syscall"

// detachedProcess is DETACHED_PROCESS: no console, neither git's nor a new
// one that would flash up on every commit.
const detachedProcess = 0x00000008

func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP}
}
