package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type containerMap map[string]*types.ContainerJSON

// ShellCmd represents a shell command to be run
type ShellCmd struct {
	Env           map[string]string
	Command       *exec.Cmd
	CommandString string
	Args          []string
	ShowOutput    bool
	Error         error
}

const APIVERSION = "1.40"
const DEBUG = true

var cm containerMap
var dockerClient *client.Client

// NewShellCmd returns a new ShellCmd struct
func NewShellCmd(command string) *ShellCmd {
	items := strings.Split(command, " ")
	cmd := items[0]
	args := items[1:]
	return &ShellCmd{
		Command:       exec.Command(cmd, args...),
		CommandString: command,
		Args:          args,
		ShowOutput:    true,
	}
}

// Execute is a lightweight wrapper around exec.Command
func (sc *ShellCmd) Execute() bool {
	env := os.Environ()
	for k, v := range sc.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	sc.Command.Env = env
	if sc.ShowOutput {
		sc.Command.Stdout = os.Stdout
		sc.Command.Stderr = os.Stderr
	}
	if err := sc.Command.Run(); err != nil {
		sc.Error = err
		return false
	}
	return true
}

func runCommand(args ...string) error {
	cmd := NewShellCmd(strings.Join(args, " "))
	cmd.ShowOutput = false
	if cmd.Execute() {
		return nil
	}
	return cmd.Error
}

func watchEvents(ctx context.Context) {
	cm = containerMap{}

	filters := filters.NewArgs(
		filters.Arg("type", events.ContainerEventType),
	)
	events, errors := dockerClient.Events(ctx, types.EventsOptions{
		Filters: filters,
	})

	for {
		select {
			case err := <-errors:
				log.Fatal().
					Err(err).
					Msg("events_failure")
			case event := <-events:
				handleEvent(ctx, event)
		}
	}
}

func handleEvent(ctx context.Context, event events.Message) (error) {
	// handle removing deleted/destroyed containers
	if event.Status == "delete" || event.Status == "destroy" {
		if _, ok := cm[event.ID]; ok {
			log.Info().
				Str("container_id", event.ID[0:9]).
				Msg("dead_container")
			delete(cm, event.ID)
		}

		return nil
	}

	container, err := dockerClient.ContainerInspect(ctx, event.ID)
	if err != nil {
		return err
	}

	appName, _ := container.Config.Labels["com.dokku.app-name"]
	if appName == "" {
		return nil
	}

	if event.Status == "die" {
		if container.HostConfig.RestartPolicy.Name == "no" {
			return nil
		}

		if container.RestartCount == container.HostConfig.RestartPolicy.MaximumRetryCount {
			log.Info().
				Str("container_id", event.ID[0:9]).
				Str("app", appName).
				Str("restart_policy", container.HostConfig.RestartPolicy.Name).
				Int("restart_count", container.RestartCount).
				Int("max_restart_count", container.HostConfig.RestartPolicy.MaximumRetryCount).
				Msg("rebuilding_app")
			if err := runCommand("dokku", "--quiet", "ps:rebuild", appName); err != nil {
				log.Warn().
					Str("container_id", event.ID[0:9]).
					Str("app", appName).
					Str("error", err.Error()).
					Msg("rebuild_failed")
				return err
			}
		}
	}

	// skip non-start events
	if event.Status != "start" && event.Status != "restart" {
		return nil
	}

	if _, ok := cm[event.ID]; !ok {
		cm[event.ID] = &container
		log.Info().
			Str("container_id", event.ID[0:9]).
			Str("app", appName).
			Str("ip_address", container.NetworkSettings.Networks["bridge"].IPAddress).
			Msg("new_container")
		return nil
	}

	existingContainer := cm[event.ID]
	cm[event.ID] = &container

	// do nothing if the ip addresses match
	if existingContainer.NetworkSettings.Networks["bridge"].IPAddress == container.NetworkSettings.Networks["bridge"].IPAddress {
		return nil
	}

	log.Info().
		Str("container_id", event.ID[0:9]).
		Str("app", appName).
		Str("old_ip_address", existingContainer.NetworkSettings.Networks["bridge"].IPAddress).
		Str("new_ip_address", container.NetworkSettings.Networks["bridge"].IPAddress).
		Msg("reloading_nginx")

	if err := runCommand("dokku", "--quiet", "nginx:build-config", appName); err != nil {
		log.Warn().
			Str("container_id", event.ID[0:9]).
			Str("app", appName).
			Str("error", err.Error()).
			Msg("reload_failed")
	}
	return err
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	var err error
	dockerClient, err = client.NewClientWithOpts(client.WithVersion(APIVERSION))
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("api_connect_failed")
	}
	ctx := context.Background()
	watchEvents(ctx)
}
