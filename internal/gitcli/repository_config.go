package gitcli

import (
	"context"
	"fmt"
	"strings"
)

type configValue struct {
	origin, key, value string
}

func (r *Repository) readConfigValues(ctx context.Context, pattern string) ([]configValue, error) {
	raw, err := r.readLimited(ctx, "", 4<<20, "config", "--null", "--show-origin", "--get-regexp", pattern)
	if gitExitCode(err, 1) {
		return nil, nil
	}
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("invalid Git config data")
	}
	values := make([]configValue, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		key, value, _ := strings.Cut(fields[i+1], "\n")
		values = append(values, configValue{fields[i], key, value})
	}
	return values, nil
}
