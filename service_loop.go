// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"CuckooInterface/core/service"
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
)

// RunServiceLoop 以 Windows 服务方式运行 Core。
// 服务由 SCM 启动，不创建托盘，也不显示 UI。
func RunServiceLoop() {
	if err := svc.Run(service.Name, &coreService{}); err != nil {
		fmt.Fprintf(os.Stderr, "run service failed: %v\n", err)
		os.Exit(1)
	}
}

type coreService struct{}

func (s *coreService) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		runMainLoop(ctx, true)
		close(done)
	}()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	for {
		select {
		case <-done:
			return false, 0
		case request, ok := <-requests:
			if !ok {
				cancel()
				<-done
				return false, 0
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
				return false, 0
			}
		}
	}
}
