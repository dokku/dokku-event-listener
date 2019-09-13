package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/docker/pkg/jsonmessage"
)

type Config struct {
	Labels map[string]string
}

type HostConfig struct {
	RestartPolicy RestartPolicy
}

type NetworkSettings struct {
	IpAddress string
}

type RestartPolicy struct {
	Name              string
	MaximumRetryCount int
}

type Container struct {
	Config          Config
	Event           jsonmessage.JSONMessage
	HostConfig      HostConfig
	ID              string
	Name            string
	NetworkSettings NetworkSettings
	RestartCount    int
}

type containerMap map[string]*Container

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

func request(path string) (*http.Response, error) {
	apiPath := fmt.Sprintf("/v%s%s", APIVERSION, path)
	req, err := http.NewRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial("unix", "/var/run/docker.sock")
	if err != nil {
		return nil, err
	}

	clientconn := httputil.NewClientConn(conn, nil)
	resp, err := clientconn.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if len(body) == 0 {
			return nil, fmt.Errorf("Error: %s", http.StatusText(resp.StatusCode))
		}

		return nil, fmt.Errorf("HTTP %s: %s", http.StatusText(resp.StatusCode), body)
	}
	return resp, nil
}

func runCommand(args ...string) error {
	cmd := NewShellCmd(strings.Join(args, " "))
	cmd.ShowOutput = false
	if cmd.Execute() {
		return nil
	}
	return cmd.Error
}

func getContainer(event jsonmessage.JSONMessage) (*Container, error) {
	resp, err := request("/containers/" + event.ID + "/json")
	if err != nil {
		return nil, fmt.Errorf("Couldn't find container for event %#v: %s", event, err)
	}
	defer resp.Body.Close()

	container := &Container{}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	container.Event = event
	container.ID = event.ID
	return container, json.Unmarshal(body, &container)
}

func watch(r io.Reader) {
	cm = containerMap{}

	dec := json.NewDecoder(r)
	for {
		event := jsonmessage.JSONMessage{}
		if err := dec.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("msg=bad_message error=%s", err)
		}

		// skip non-container messages
		if event.ID == "" {
			continue
		}

		// handle removing deleted/destroyed containers
		if event.Status == "delete" || event.Status == "destroy" {
			if _, ok := cm[event.ID]; ok {
				log.Printf("msg=dead_container container_id=%v", event.ID[0:9])
				delete(cm, event.ID)
			}

			continue
		}

		container, err := getContainer(event)
		if err != nil {
			continue
		}

		// log.Printf("Got container: %+v", container)
		// log.Printf("Got event: %+v", event)
		appName, _ := container.Config.Labels["com.dokku.app-name"]
		if appName == "" {
			continue
		}

		if event.Status == "die" {
			if container == nil {
				continue
			}

			if container.HostConfig.RestartPolicy.Name == "no" {
				continue
			}

			if container.RestartCount == container.HostConfig.RestartPolicy.MaximumRetryCount {
				log.Printf("msg=rebuilding_app container_id=%v app=%v restart_policy=%v restart_count=%v max_restart_count=%v", event.ID[0:9], appName, container.HostConfig.RestartPolicy.Name, container.RestartCount, container.HostConfig.RestartPolicy.MaximumRetryCount)
				if err := runCommand("dokku", "--quiet", "ps:rebuild", appName); err != nil {
					log.Printf("msg=rebuild_failed container_id=%v app=%v error=%v", event.ID[0:9], appName, err)
				}
			}
		}

		// skip non-start events
		if event.Status != "start" && event.Status != "restart" {
			continue
		}

		if _, ok := cm[event.ID]; !ok {
			log.Printf("msg=new_container container_id=%v app=%v", event.ID[0:9], appName)
			cm[event.ID] = container
			continue
		}

		existingContainer := cm[event.ID]
		cm[event.ID] = container

		// do nothing if the ip addresses match
		if existingContainer.NetworkSettings.IpAddress == container.NetworkSettings.IpAddress {
			continue
		}

		log.Printf("msg=reloading_nginx container_id=%v app=%v old_ip_address=%v new_ip_address=%v", event.ID[0:9], appName, existingContainer.NetworkSettings.IpAddress, container.NetworkSettings.IpAddress)
		if err := runCommand("dokku", "--quiet", "nginx:build-config", appName); err != nil {
			log.Printf("msg=reload_failed container_id=%v app=%v error=%v", event.ID[0:9], appName, err)
		}
	}
}

func main() {
	resp, err := request("/events")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	watch(resp.Body)
}
