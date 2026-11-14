package main

import (
	"flag"
	"sync"
)

var args []string = []string{
	"launch_type",
	"no_ui",
}

// 启动类型
var launchType string

// 是否启用ui
var isUi bool

var argOnce sync.Once

func init() {
	lt := flag.String(args[0], "daemon", "select launch type")
	ui := flag.Bool(args[1], false, "select ui stat")
	flag.Parse()
	argOnce.Do(func() {
		launchType = *lt
		isUi = !*ui
	})
}
