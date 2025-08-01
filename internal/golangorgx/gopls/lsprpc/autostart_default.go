// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lsprpc

import (
	"fmt"
	"os/exec"
)

var (
	daemonize             = func(*exec.Cmd) {}
	autoNetworkAddress    = autoNetworkAddressDefault
	verifyRemoteOwnership = verifyRemoteOwnershipDefault
)

func runRemote(cmd *exec.Cmd) error {
	daemonize(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting remote cuelsp: %w", err)
	}
	return nil
}

// autoNetworkAddressDefault returns the default network and address for the
// automatically-started cuelsp remote. See autostart_posix.go for more
// information.
func autoNetworkAddressDefault(cuePath, id string) (network string, address string) {
	if id != "" {
		panic("identified remotes are not supported on windows")
	}
	return "tcp", "localhost:37375"
}

func verifyRemoteOwnershipDefault(network, address string) (bool, error) {
	return true, nil
}
