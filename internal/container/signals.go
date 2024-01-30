package container

import (
	"context"
	"os"

	"github.com/docker/docker/client"
	"github.com/moby/sys/signal"

	"code-intelligence.com/cifuzz/pkg/log"
)

/*
Based on ForwardAllSignals from github.com/docker/cli/cli/command/container/signals.go
https://github.com/docker/cli/blob/e0e27724390cbce21cf6d67568972a4227b07382/cli/command/container/signals.go#L16

Copyright 2012-2017 Docker, Inc.
Apache License 2.0
*/
func forwardAllSignals(ctx context.Context, cli *client.Client, id string, sigc <-chan os.Signal) {
	var (
		s  os.Signal
		ok bool
	)
	for {
		select {
		case s, ok = <-sigc:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}

		if s == signal.SIGCHLD || s == signal.SIGPIPE {
			continue
		}

		// In go1.14+, the go runtime issues SIGURG as an interrupt to support pre-emptable system calls on Linux.
		// Since we can't forward that along we'll check that here.
		if isRuntimeSig(s) {
			continue
		}
		var sig string
		for sigStr, sigN := range signal.SignalMap {
			if sigN == s {
				sig = sigStr
				break
			}
		}
		if sig == "" {
			continue
		}

		if err := cli.ContainerKill(ctx, id, sig); err != nil {
			log.Debugf("Error sending signal: %s", err)
		}
	}
}
