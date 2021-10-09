package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

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

const APIVERSION = "1.25"
const DEBUG = true
const DOKKU_APP_LABEL = "com.dokku.app-name"

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

func registerContainers(ctx context.Context) error {
	cm = containerMap{}
	filters := filters.NewArgs(
		filters.Arg("label", DOKKU_APP_LABEL),
	)
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters,
	})
	if err != nil {
		return err
	}

	for _, container := range containers {
		containerJSON, err := dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			return err
		}
		if _, ok := containerJSON.NetworkSettings.Networks["bridge"]; !ok {
			log.Info().
				Str("container_id", container.ID[0:9]).
				Str("app", containerJSON.Config.Labels[DOKKU_APP_LABEL]).
				Msg("register_skip:non-bridge-network")
			continue
		}

		cm[container.ID] = &containerJSON
		log.Info().
			Str("container_id", container.ID[0:9]).
			Str("app", containerJSON.Config.Labels[DOKKU_APP_LABEL]).
			Str("ip_address", containerJSON.NetworkSettings.Networks["bridge"].IPAddress).
			Msg("register_container")
	}
	return nil
}

func watchEvents(ctx context.Context, sinceTimestamp int64) {
	filters := filters.NewArgs(
		filters.Arg("type", events.ContainerEventType),
		filters.Arg("label", DOKKU_APP_LABEL),
	)
	events, errors := dockerClient.Events(ctx, types.EventsOptions{
		Since: strconv.FormatInt(sinceTimestamp, 10),
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
	containerId := event.Actor.ID
	containerShortId := containerId[0:9]

	// handle removing deleted/destroyed containers
	if event.Action == "delete" || event.Action == "destroy" {
		if _, ok := cm[containerId]; ok {
			log.Info().
				Str("container_id", containerShortId).
				Msg("dead_container")
			delete(cm, containerId)
		}

		return nil
	}

	container, err := dockerClient.ContainerInspect(ctx, containerId)
	if err != nil {
		return err
	}

	appName := container.Config.Labels[DOKKU_APP_LABEL]

	if event.Action == "die" {
		if container.HostConfig.RestartPolicy.Name == "no" {
			return nil
		}

		if container.RestartCount == container.HostConfig.RestartPolicy.MaximumRetryCount {
			log.Info().
				Str("container_id", containerShortId).
				Str("app", appName).
				Str("restart_policy", container.HostConfig.RestartPolicy.Name).
				Int("restart_count", container.RestartCount).
				Int("max_restart_count", container.HostConfig.RestartPolicy.MaximumRetryCount).
				Msg("rebuilding_app")
			if err := runCommand("dokku", "--quiet", "ps:rebuild", appName); err != nil {
				log.Warn().
					Str("container_id", containerShortId).
					Str("app", appName).
					Str("error", err.Error()).
					Msg("rebuild_failed")
				return err
			}
		}
	}

	// skip non-start events
	if event.Action != "start" && event.Action != "restart" {
		return nil
	}

	if _, ok := container.NetworkSettings.Networks["bridge"]; !ok {
		log.Info().
			Str("container_id", containerShortId).
			Str("app", appName).
			Msg("non-bridge-network")
		return nil
	}

	if _, ok := cm[containerId]; !ok {
		cm[containerId] = &container
		log.Info().
			Str("container_id", containerShortId).
			Str("app", appName).
			Str("ip_address", container.NetworkSettings.Networks["bridge"].IPAddress).
			Msg("new_container")
		return nil
	}

	existingContainer := cm[containerId]
	cm[containerId] = &container

	// do nothing if the ip addresses match
	if existingContainer.NetworkSettings.Networks["bridge"].IPAddress == container.NetworkSettings.Networks["bridge"].IPAddress {
		return nil
	}

	log.Info().
		Str("container_id", containerShortId).
		Str("app", appName).
		Str("old_ip_address", existingContainer.NetworkSettings.Networks["bridge"].IPAddress).
		Str("new_ip_address", container.NetworkSettings.Networks["bridge"].IPAddress).
		Msg("reloading_nginx")

	if err := runCommand("dokku", "--quiet", "nginx:build-config", appName); err != nil {
		log.Warn().
			Str("container_id", containerShortId).
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
	startupTimestamp := time.Now().Unix()
	if err := registerContainers(ctx); err != nil {
		log.Fatal().
			Err(err).
			Msg("containers_init_failed")
	}
	watchEvents(ctx, startupTimestamp)
}
