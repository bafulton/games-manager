// Copyright 2026 games-manager contributors.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

// Run executes cmd from the project root and returns its combined output.
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO111MODULE=on")

	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}
	return string(output), nil
}

// Kubectl runs kubectl with args and returns its trimmed output.
func Kubectl(args ...string) (string, error) {
	out, err := Run(exec.Command("kubectl", args...))
	return strings.TrimSpace(out), err
}

// LoadImageToKindClusterWithName loads a local docker image into the kind
// cluster named by KIND_CLUSTER (default "kind").
func LoadImageToKindClusterWithName(name string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	_, err := Run(exec.Command("kind", "load", "docker-image", name, "--name", cluster))
	return err
}

// GetProjectDir returns the repository root.
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	return strings.ReplaceAll(wd, "/test/e2e", ""), nil
}
