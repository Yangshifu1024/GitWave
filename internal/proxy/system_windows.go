package proxy

import (
	"context"

	"golang.org/x/sys/windows/registry"
)

func detectSystem(context.Context) systemProxy {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return systemProxy{}
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return systemProxy{}
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return systemProxy{}
	}
	bypass, _, _ := key.GetStringValue("ProxyOverride")
	return windowsProxy(server, bypass)
}
