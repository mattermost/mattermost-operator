// plugin-manager is a small binary injected into Mattermost pods to manage
// plugin lifecycle via mmctl local mode. It has two sub-commands:
//
//	copy-self <dest>  – copies this binary to <dest> (used by an init container)
//	run               – waits for the mmctl socket then reconciles plugins
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// PluginSpec mirrors apis/mattermost/v1beta1.PluginSpec.
type PluginSpec struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
	Enabled bool   `json:"enabled"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: plugin-manager <copy-self|run> [args...]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "copy-self":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: plugin-manager copy-self <dest>")
			os.Exit(1)
		}
		if err := copySelf(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "copy-self: %v\n", err)
			os.Exit(1)
		}
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		socketPath := fs.String("socket", "/run/mattermost/mattermost_local.socket", "path to mmctl local socket")
		mmctlBin := fs.String("mmctl", "/mattermost/bin/mmctl", "path to mmctl binary")
		pluginsJSON := fs.String("plugins", "[]", "JSON-encoded list of PluginSpec")
		if err := fs.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "run: %v\n", err)
			os.Exit(1)
		}

		var plugins []PluginSpec
		if err := json.Unmarshal([]byte(*pluginsJSON), &plugins); err != nil {
			fmt.Fprintf(os.Stderr, "run: invalid --plugins JSON: %v\n", err)
			os.Exit(1)
		}
		runManager(*socketPath, *mmctlBin, plugins)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func copySelf(dest string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve self: %w", err)
	}
	src, err := os.Open(self)
	if err != nil {
		return fmt.Errorf("open self: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("open dest %s: %w", dest, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

func mmctlCmd(mmctlBin, configPath string, args ...string) *exec.Cmd {
	allArgs := append([]string{"--config", configPath}, args...)
	cmd := exec.Command(mmctlBin, allArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func writeMMCtlConfig(socketPath string) (string, error) {
	f, err := os.CreateTemp("", "mmctl-config-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, `{"ServiceSettings":{"LocalModeSocketLocation":%q}}`, socketPath)
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func runManager(socketPath, mmctlBin string, plugins []PluginSpec) {
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		log.Println("plugin-manager: waiting for socket...")
		time.Sleep(2 * time.Second)
	}
	log.Println("plugin-manager: socket ready, reconciling plugins...")

	configPath, err := writeMMCtlConfig(socketPath)
	if err != nil {
		log.Printf("plugin-manager: failed to write mmctl config: %v", err)
		return
	}
	defer os.Remove(configPath)

	out, _ := mmctlCmd(mmctlBin, configPath, "--local", "plugin", "list").Output()
	installed := string(out)

	for _, p := range plugins {
		needsInstall := !strings.Contains(installed, p.ID+": ") ||
			!strings.Contains(installed, "Version: "+p.Version)

		if needsInstall {
			var cmd *exec.Cmd
			if p.URL != "" {
				cmd = mmctlCmd(mmctlBin, configPath, "--local", "plugin", "install-url", "--force", p.URL)
			} else {
				cmd = mmctlCmd(mmctlBin, configPath, "--local", "marketplace", "install", p.ID)
			}
			if err := cmd.Run(); err != nil {
				log.Printf("plugin-manager: failed to install %s: %v", p.ID, err)
			}
		}

		action := "enable"
		enableCmd := mmctlCmd(mmctlBin, configPath, "--local", "plugin", "enable", p.ID)
		if !p.Enabled {
			action = "disable"
			enableCmd = mmctlCmd(mmctlBin, configPath, "--local", "plugin", "disable", p.ID)
		}
		if err := enableCmd.Run(); err != nil {
			log.Printf("plugin-manager: failed to %s %s: %v", action, p.ID, err)
		}
	}

	log.Println("plugin-manager: reconciliation complete")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	<-sigs
}
