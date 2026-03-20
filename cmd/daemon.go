package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/lunguini/gocker/api"
	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func gockerDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gocker")
}

func newDaemonCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:  "daemon",
		Usage: "Manage the gocker API daemon",
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the API daemon",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "foreground", Usage: "Run in foreground"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					dir := gockerDir()
					os.MkdirAll(dir, 0755)

					socketPath := filepath.Join(dir, "gocker.sock")
					pidPath := filepath.Join(dir, "daemon.pid")

					if cmd.Bool("foreground") {
						pid := os.Getpid()
						os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644)
						defer os.Remove(pidPath)
						defer os.Remove(socketPath)

						fmt.Printf("Starting gocker daemon (pid %d)\n", pid)
						fmt.Printf("Listening on %s\n", socketPath)
						srv := api.NewServer(eng, socketPath)
						return srv.ListenAndServe(ctx)
					}

					// Background mode: re-exec ourselves
					exe, err := os.Executable()
					if err != nil {
						return fmt.Errorf("finding executable: %w", err)
					}
					proc, err := os.StartProcess(exe,
						[]string{exe, "daemon", "start", "--foreground"},
						&os.ProcAttr{
							Dir:   "/",
							Env:   os.Environ(),
							Files: []*os.File{nil, nil, nil},
							Sys:   &syscall.SysProcAttr{Setsid: true},
						},
					)
					if err != nil {
						return fmt.Errorf("starting daemon: %w", err)
					}
					fmt.Printf("Daemon started (pid %d)\n", proc.Pid)
					fmt.Printf("Socket: %s\n", socketPath)
					proc.Release()
					return nil
				},
			},
			{
				Name:  "stop",
				Usage: "Stop the API daemon",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					pidPath := filepath.Join(gockerDir(), "daemon.pid")
					data, err := os.ReadFile(pidPath)
					if err != nil {
						return fmt.Errorf("daemon not running (no pid file)")
					}
					pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
					if err != nil {
						return fmt.Errorf("invalid pid file")
					}
					proc, err := os.FindProcess(pid)
					if err != nil {
						return fmt.Errorf("finding process: %w", err)
					}
					if err := proc.Signal(syscall.SIGTERM); err != nil {
						return fmt.Errorf("stopping daemon: %w", err)
					}
					os.Remove(pidPath)
					fmt.Println("Daemon stopped")
					return nil
				},
			},
			{
				Name:  "status",
				Usage: "Show daemon status",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					pidPath := filepath.Join(gockerDir(), "daemon.pid")
					data, err := os.ReadFile(pidPath)
					if err != nil {
						fmt.Println("Daemon is not running")
						return nil
					}
					pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
					if err != nil {
						fmt.Println("Daemon is not running (invalid pid file)")
						return nil
					}
					proc, err := os.FindProcess(pid)
					if err != nil {
						fmt.Println("Daemon is not running")
						return nil
					}
					if err := proc.Signal(syscall.Signal(0)); err != nil {
						fmt.Println("Daemon is not running (stale pid file)")
						return nil
					}
					socketPath := filepath.Join(gockerDir(), "gocker.sock")
					fmt.Printf("Daemon is running (pid %d)\n", pid)
					fmt.Printf("Socket: %s\n", socketPath)
					return nil
				},
			},
		},
	}
}
