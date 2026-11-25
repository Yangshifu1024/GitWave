package proxy

import (
	"context"
	"os/exec"
	"time"
)

func detectSystem(ctx context.Context) systemProxy {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return systemProxy{}
	}
	return macProxy(string(output))
}
