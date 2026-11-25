//go:build !darwin && !windows

package proxy

import "context"

func detectSystem(context.Context) systemProxy { return systemProxy{} }
