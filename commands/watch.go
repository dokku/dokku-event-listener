package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/josegonzalez/cli-skeleton/command"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/posener/complete"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	flag "github.com/spf13/pflag"
)

type containerMap map[string]*container.InspectResponse

// ShellCmd represents a shell command to be run
type ShellCmd struct {
	Env           map[string]string
	Command       *exec.Cmd
	CommandString string
	Args          []string
	ShowOutput    bool
	Error         error
}

const DEBUG = true
const DOKKU_APP_LABEL = "com.dokku.app-name"
const DOKKU_PROCESS_TYPE_LABEL = "com.dokku.process-type=web"

var cm containerMap
var dockerClient *client.Client

type WatchCommand struct {
	command.Meta
}

func (c *WatchCommand) Name() string {
	return "watch"
}

func (c *WatchCommand) Synopsis() string {
	return "Watches Dokku containers and restarts the proxy as necessary"
}

func (c *WatchCommand) Help() string {
	return command.CommandHelp(c)
}

func (c *WatchCommand) Examples() map[string]string {
	appName := os.Getenv("CLI_APP_NAME")
	return map[string]string{
		"Watch containers run": fmt.Sprintf("%s %s", appName, c.Name()),
	}
}

func (c *WatchCommand) Arguments() []command.Argument {
	args := []command.Argument{}
	return args
}

func (c *WatchCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *WatchCommand) ParsedArguments(args []string) (map[string]command.Argument, error) {
	return command.ParseArguments(args, c.Arguments())
}

func (c *WatchCommand) FlagSet() *flag.FlagSet {
	f := c.Meta.FlagSet(c.Name(), command.FlagSetClient)
	return f
}

func (c *WatchCommand) AutocompleteFlags() complete.Flags {
	return command.MergeAutocompleteFlags(
		c.Meta.AutocompleteFlags(command.FlagSetClient),
		complete.Flags{},
	)
}

func (c *WatchCommand) Run(args []string) int {
	flags := c.FlagSet()
	flags.Usage = func() { c.Ui.Output(c.Help()) }
	if err := flags.Parse(args); err != nil {
		c.Ui.Error(err.Error())
		c.Ui.Error(command.CommandErrorText(c))
		return 1
	}

	_, err := c.ParsedArguments(flags.Args())
	if err != nil {
		c.Ui.Error(err.Error())
		c.Ui.Error(command.CommandErrorText(c))
		return 1
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	ctx := context.Background()
	sinceTimestamp := time.Now().Unix()

	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 60 * time.Second
		backoffFactor  = 2
	)
	backoff := initialBackoff

	for {
		log.Info().Msg("connecting_to_docker")
		start := time.Now()
		err := reconnectAndWatch(ctx, &sinceTimestamp)
		if err != nil {
			if time.Since(start) > 30*time.Second {
				backoff = initialBackoff
			}
			log.Error().
				Err(err).
				Dur("retry_in", backoff).
				Msg("docker_disconnected")
			time.Sleep(backoff)
			backoff *= time.Duration(backoffFactor)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		return 0
	}
}

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
	filters := make(client.Filters).
		Add("label", DOKKU_APP_LABEL, DOKKU_PROCESS_TYPE_LABEL)
	containers, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
		Filters: filters,
	})
	if err != nil {
		return err
	}

	for _, container := range containers.Items {
		inspect, err := dockerClient.ContainerInspect(ctx, container.ID, client.ContainerInspectOptions{})
		if err != nil {
			return err
		}
		containerJSON := inspect.Container
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
			Str("ip_address", containerJSON.NetworkSettings.Networks["bridge"].IPAddress.String()).
			Msg("register_container")
	}
	return nil
}

func reconnectAndWatch(ctx context.Context, sinceTimestamp *int64) error {
	var err error
	dockerClient, err = client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("api_connect_failed: %w", err)
	}
	defer dockerClient.Close()

	if err := registerContainers(ctx); err != nil {
		return fmt.Errorf("containers_init_failed: %w", err)
	}

	log.Info().Msg("connected_to_docker")

	return watchEvents(ctx, sinceTimestamp)
}

func watchEvents(ctx context.Context, sinceTimestamp *int64) error {
	filters := make(client.Filters).
		Add("type", string(events.ContainerEventType)).
		Add("label", DOKKU_APP_LABEL, DOKKU_PROCESS_TYPE_LABEL)
	stream := dockerClient.Events(ctx, client.EventsListOptions{
		Since:   strconv.FormatInt(*sinceTimestamp, 10),
		Filters: filters,
	})

	for {
		select {
		case err := <-stream.Err:
			log.Error().Err(err).Msg("events_stream_error")
			return err
		case event, ok := <-stream.Messages:
			if !ok {
				return fmt.Errorf("events channel closed")
			}
			handleEvent(ctx, event)
			if event.Time > 0 {
				*sinceTimestamp = event.Time
			}
		}
	}
}

// shouldRebuildOnDie reports whether a container's "die" event means Docker has
// permanently given up restarting it, so the app should be rebuilt. This only
// happens with the "on-failure" restart policy once the restart count reaches a
// positive maximum retry count. Containers using "always"/"unless-stopped" are
// restarted by Docker indefinitely and "no" containers are never restarted, so
// rebuilding them here would create an infinite loop: their MaximumRetryCount is
// always 0, which matches the RestartCount of a freshly created replacement.
func shouldRebuildOnDie(restartPolicy container.RestartPolicy, restartCount int) bool {
	return restartPolicy.IsOnFailure() &&
		restartPolicy.MaximumRetryCount > 0 &&
		restartCount >= restartPolicy.MaximumRetryCount
}

func handleEvent(ctx context.Context, event events.Message) error {
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

	inspect, err := dockerClient.ContainerInspect(ctx, containerId, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	container := inspect.Container

	appName := container.Config.Labels[DOKKU_APP_LABEL]

	if event.Action == "die" {
		restartPolicy := container.HostConfig.RestartPolicy
		if shouldRebuildOnDie(restartPolicy, container.RestartCount) {
			log.Info().
				Str("container_id", containerShortId).
				Str("app", appName).
				Str("restart_policy", string(restartPolicy.Name)).
				Int("restart_count", container.RestartCount).
				Int("max_restart_count", restartPolicy.MaximumRetryCount).
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

		return nil
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
			Str("ip_address", container.NetworkSettings.Networks["bridge"].IPAddress.String()).
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
		Str("old_ip_address", existingContainer.NetworkSettings.Networks["bridge"].IPAddress.String()).
		Str("new_ip_address", container.NetworkSettings.Networks["bridge"].IPAddress.String()).
		Msg("reloading_proxy")

	if err := runCommand("dokku", "--quiet", "proxy:build-config", appName); err != nil {
		log.Warn().
			Str("container_id", containerShortId).
			Str("app", appName).
			Str("error", err.Error()).
			Msg("reload_failed")
	}
	return err
}
