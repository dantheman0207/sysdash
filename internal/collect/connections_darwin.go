//go:build darwin

package collect

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/daniel/sysdash/internal/domain"
)

func ProcessConnections(ctx context.Context) ([]domain.ConnectionInfo, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-i", "-n", "-P")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	first := true
	var items []domain.ConnectionInfo
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		pid, _ := strconv.Atoi(fields[1])
		nameField := fields[len(fields)-1]
		local, remote, state := parseLsofName(nameField)
		items = append(items, domain.ConnectionInfo{
			Command: fields[0],
			PID:     int32(pid),
			User:    fields[2],
			Proto:   fields[7],
			Local:   local,
			Remote:  remote,
			State:   state,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].PID == items[j].PID {
			if items[i].Command == items[j].Command {
				return items[i].Local < items[j].Local
			}
			return items[i].Command < items[j].Command
		}
		return items[i].PID < items[j].PID
	})

	return items, scanner.Err()
}

func parseLsofName(name string) (local, remote, state string) {
	state = "-"
	if idx := strings.Index(name, " ("); idx >= 0 && strings.HasSuffix(name, ")") {
		state = strings.TrimSuffix(name[idx+2:], ")")
		name = name[:idx]
	}
	if strings.Contains(name, "->") {
		parts := strings.SplitN(name, "->", 2)
		return parts[0], parts[1], state
	}
	return name, "", state
}
