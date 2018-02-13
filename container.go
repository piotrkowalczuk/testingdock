package testingdock

import (
	"bufio"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	clicfg "github.com/docker/docker/cli/config"
	"github.com/docker/docker/client"
)

// HealthCheckFunc is the type of a health checking function, which is supposed
// to return nil on success, indicating that a container is not only "up", but
// "accessible" in the specified way.
//
// If the function returns an error, it will be called until it doesn't (blocking).
type HealthCheckFunc func(ctx context.Context) error

// HealthCheckHTTP is a pre-implemented HealthCheckFunc which checks if the given
// url returns http.StatusOk.
func HealthCheckHTTP(url string) HealthCheckFunc {
	return func(ctx context.Context) error {
		req, err := http.NewRequest("GET", url, nil)

		if err != nil {
			return err
		}

		req = req.WithContext(ctx)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("wrong status code: %s", http.StatusText(res.StatusCode))
		}
		return nil
	}
}

// HealthCheckCustom is just a convenience wrapper to set a HealthCheckFunc without any arguments.
func HealthCheckCustom(fn func() error) HealthCheckFunc {
	return func(ctx context.Context) error {
		return fn()
	}
}

// ResetFunc is the type of the container reset function, which is called on
// c.Reset().
type ResetFunc func(ctx context.Context, c *Container) error

// resetRestart is a pre-implemented ResetFunc, which just restarts the container.
func resetRestart() ResetFunc {
	return func(ctx context.Context, c *Container) error {
		return c.cli.ContainerRestart(ctx, c.ID, nil)
	}
}

// ResetCustom is just a convenience wrapper to set a ResetFunc.
func ResetCustom(fn func() error) ResetFunc {
	return func(ctx context.Context, c *Container) error {
		return fn()
	}
}

// ContainerOpts is an option struct for creating a docker container
// configuration.
type ContainerOpts struct {
	ForcePull bool
	// AutoRemove is always set to true
	Config     *container.Config
	HostConfig *container.HostConfig
	Name       string
	// Function called on start and reset to check whether the container
	// is 'really' up, it will block until it returns nil. The zero
	// value is a function, which just checks the docker container
	// has started.
	HealthCheck HealthCheckFunc
	// default is 30s
	HealthCheckTimeout time.Duration
	// Function called when the containers are reset. The zero value is
	// a function, which will restart the container completely.
	Reset ResetFunc
}

// Container is a docker container configuration,
// not necessarily a running or created container.
// This should usually be created via the NewContainer
// function.
type Container struct { // nolint: maligned
	t                  testing.TB
	forcePull          bool
	cli                *client.Client
	network            *Network
	ccfg               *container.Config
	hcfg               *container.HostConfig
	ID, Name, Image    string
	healthcheck        HealthCheckFunc
	healthchecktimeout time.Duration
	// children are dependencies that are started after the main container
	children []*Container
	cancel   func()
	resetF   ResetFunc
	closed   bool
}

// Creates a new container configuration with the given options.
func newContainer(t testing.TB, c *client.Client, opts ContainerOpts) *Container {
	// set default
	if opts.HealthCheckTimeout == 0 { // zero value
		opts.HealthCheckTimeout = 30 * time.Second
	}

	// always autoremove
	if opts.HostConfig == nil {
		opts.HostConfig = &container.HostConfig{}
	}
	opts.HostConfig.AutoRemove = true

	// set testingdock label
	opts.Config.Labels = createTestingLabel()

	// set default resetFunc
	if opts.Reset == nil {
		opts.Reset = resetRestart()
	}

	cont := &Container{
		t:                  t,
		forcePull:          opts.ForcePull,
		Name:               opts.Name,
		healthcheck:        opts.HealthCheck,
		healthchecktimeout: opts.HealthCheckTimeout,
		cli:                c,
		ccfg:               opts.Config,
		hcfg:               opts.HostConfig,
		resetF:             opts.Reset,
	}

	// set default healthcheck
	if opts.HealthCheck == nil {
		cont.healthcheck = cont.healthCheckRunning()
	}

	return cont
}

// start actually starts a docker container. This may also pull images.
func (c *Container) start(ctx context.Context) { // nolint: gocyclo
	if c.network == nil {
		c.t.Fatalf("Container %s not added to any network!", c.Name)
	}

	imageListArgs := filters.NewArgs()
	imageListArgs.Add("reference", c.ccfg.Image)

	images, err := c.cli.ImageList(ctx, types.ImageListOptions{Filters: imageListArgs})
	if err != nil {
		c.t.Fatalf("image listing failure: %s", err.Error())
	}

	if len(images) == 0 || c.forcePull {
		printf("(setup) %-25s - pulling image", c.ccfg.Image)
		img, err := c.imagePull(ctx)
		if err != nil {
			c.t.Fatalf("image downloading failure of '%s': %s", c.ccfg.Image, err.Error())
		}
		if _, err = io.Copy(ioutil.Discard, img); err != nil {
			c.t.Fatalf("image pull response read failure: %s", err.Error())
		}
		if err = img.Close(); err != nil {
			c.t.Fatalf("image closing failure: %s", err.Error())
		}
		printf("(setup) %-25s - successfully pulled image", c.ccfg.Image)
	}

	c.initialCleanup(ctx)

	hcfg := *c.hcfg
	hcfg.NetworkMode = container.NetworkMode(c.network.name)

	cont, err := c.cli.ContainerCreate(ctx, c.ccfg, &hcfg, nil, c.Name)
	if err != nil {
		c.t.Fatalf("container creation failure: %s", err.Error())
	}

	c.ID = cont.ID

	c.cancel = func() {
		if c.closed {
			return
		}
		if err := c.cli.NetworkDisconnect(ctx, c.network.id, c.ID, true); err != nil {
			c.t.Fatalf("container disconnect failure: %s", err.Error())
		}
		printf("(cancel) %-25s (%s) - container disconnected from: %s", c.Name, c.ID, c.network.name)
		if err := c.cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
			c.t.Fatalf("container removal failure: %s", err.Error())
		}
		printf("(cancel) %-25s (%s) - container removed", c.Name, c.ID)
	}

	// start the container finally
	if err = c.cli.ContainerStart(ctx, c.ID, types.ContainerStartOptions{}); err != nil {
		c.t.Fatalf("container start failure: %s", err.Error())
	}

	printf("(setup ) %-25s (%s) - container started", c.Name, c.ID)

	// start container logging
	if Verbose {
		go func() {
			reader, gerr := c.cli.ContainerLogs(ctx, cont.ID, types.ContainerLogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Follow:     true,
			})
			if gerr != nil {
				c.t.Fatalf("container logging failure: %s", gerr.Error())
			}
			printf("(loggi ) %-25s (%s) - container logging started", c.Name, c.ID)

			scanner := bufio.NewScanner(reader)
			for scanner.Scan() { // scanner loop
				if line := scanner.Text(); len(line) > 0 {
					printf("(clogs ) %-25s (%s) - %s", c.Name, c.ID, line)

				}
			}

			serr := scanner.Err()
			if serr != nil && serr != io.EOF {
				c.t.Fatalf("container logging failure: %s", serr.Error())
			} else {
				printf("(loggi ) %-25s (%s) - %s", c.Name, c.ID, "EOF reached, stopping logging")
				return // io.EOF, stop goroutine
			}
		}()
	}

	c.executeHealthCheck(ctx)

	// start children
	if SpawnSequential {
		for _, cont := range c.children {
			cont.start(ctx)
		}
	} else {
		printf("(setup ) %-25s (%s) - container is spawning %d child containers in parallel", c.Name, c.ID, len(c.children))

		var wg sync.WaitGroup

		wg.Add(len(c.children))
		for _, cont := range c.children {
			go func(cont *Container) {
				defer wg.Done()
				cont.start(ctx)
			}(cont)
		}
		wg.Wait()
	}
}

// Find containers by the given name.
func findContainerByName(ctx context.Context, cli *client.Client, name string) ([]types.Container, error) {
	containerListArgs := filters.NewArgs()
	containerListArgs.Add("name", name)
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		Filters: containerListArgs,
	})
	if err != nil {
		return nil, err
	}

	return containers, nil
}

// Removes already existing containers with the same name as the
// the current Container configuration. Only containers with the
// label "owner=testingdock" are removed.
func (c *Container) initialCleanup(ctx context.Context) {
	containers, err := findContainerByName(ctx, c.cli, c.Name)
	if err != nil {
		c.t.Fatalf("container listing failure: %s", err.Error())
	}
	for _, cont := range containers {
		if isOwnedByTestingdock(cont.Labels) {
			if err = c.cli.ContainerRemove(ctx, cont.ID, types.ContainerRemoveOptions{
				Force:         true,
				RemoveVolumes: true,
			}); err != nil {
				c.t.Fatalf("container removal failure: %s", err.Error())
			}
			printf("(setup ) %-25s (%s) - container removed", cont.Names[0], cont.ID)
		} else {
			c.t.Fatalf("container with name %s already exists, but wasn't started by tesingdock, aborting!", c.Name)
		}
	}
}

// Closes a container and its children. This calls the
// 'cancel' function set in the Container struct.
// Implements io.Closer interface.
func (c *Container) close() error {
	if SpawnSequential {
		for _, cont := range c.children {
			cont.close() // nolint: errcheck
		}
	} else {
		var wg sync.WaitGroup

		wg.Add(len(c.children))
		for _, cont := range c.children {
			go func(cont *Container) {
				defer wg.Done()
				cont.close() // nolint: errcheck
			}(cont)
		}
		wg.Wait()
	}

	c.cancel()
	c.closed = true
	return nil
}

// After adds a child container (dependency, sort of)
// to the current container configuration in the same network.
func (c *Container) After(cc *Container) {
	cc.network = c.network
	c.children = append(c.children, cc)
}

// Calls the ResetFunc set in the Container struct for the
// whole configuration, including children containers.
// Aborts early if there is any error during reset.
func (c *Container) reset(ctx context.Context) {
	if err := c.resetF(ctx, c); err != nil {
		c.t.Fatalf("container reset failure: %s", err.Error())
	}
	c.executeHealthCheck(ctx)

	for _, cc := range c.children {
		cc.reset(ctx)
	}

	printf("(reset ) %-25s (%s) - container reset", c.Name, c.ID)
}

// Blocks until either the healthcheck returns no error or the context
// is cancelled.
func (c *Container) executeHealthCheck(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, c.healthchecktimeout)
	defer cancel()
InfLoop:
	for {
		select {
		case <-ctx.Done():
			c.t.Fatalf("health check failure: %s", ctx.Err())
		case <-time.After(1 * time.Second):
			if err := c.healthcheck(ctx); err != nil {
				printf("(setup ) %-25s (%s) - container health failure: %s", c.Name, c.ID, err.Error())
				continue InfLoop
			}
			break InfLoop
		}
	}
}

// wrapper around cli.ImagePull to fill ImagePullOptions with authentication information, if any.
func (c *Container) imagePull(ctx context.Context) (io.ReadCloser, error) {
	pullOptions := types.ImagePullOptions{}

	// https://github.com/docker/distribution/blob/master/reference/reference.go#L7
	//
	// First part of the name *could* be a domain. If there is a corresponding entry in the
	// .docker/config.json, it probably is.
	//
	// There is an undocumented hack to determine whether the first component is an actual domain, but it's
	// shit: https://github.com/docker/distribution/blob/545102ea07aa9796f189d82f606b7c27d7aa3ed3/reference/normalize.go#L62
	nameParts := strings.SplitN(c.ccfg.Image, "/", 2)

	// get the credentials
	if len(nameParts) >= 2 { // e.g.: quay.io/hans/myimage:latest
		domain := nameParts[0]

		token, err := getCredentialsFromConfig(domain)

		// if err is non-nil, then we couldn't get credentials,
		// because either it wasn't a domain or the user did not log in
		if err == nil {
			pullOptions.RegistryAuth = token
		} else {
			printf("(setup) %-25s - failed to get credentials, not fatal (%s)", c.ccfg.Image, err)
		}
	}

	return c.cli.ImagePull(ctx, c.ccfg.Image, pullOptions)
}

// get credentials from ~/.docker/config.json
func getCredentialsFromConfig(domain string) (string, error) {
	cfg, err := clicfg.Load(clicfg.Dir())
	if err != nil {
		return "", err
	}

	dcfg, ok := cfg.AuthConfigs[domain]

	if !ok {
		return "", fmt.Errorf("domain %s does not exist in config", domain)
	}

	if dcfg.Password == "" {
		return "", fmt.Errorf("no password set")
	}

	type SecToken struct {
		username string
		password string
	}
	token := SecToken{
		username: dcfg.Username,
		password: dcfg.Password,
	}
	var jsonToken []byte
	jsonToken, err = json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("internal error: failed to marshal json: %s", err)
	}

	return b64.StdEncoding.EncodeToString(jsonToken), nil
}

// Check if the container is running. If ContainerInspect fails at any point, assume
// the container is not running.
func containerIsRunning(ctx context.Context, cli *client.Client, id string) bool {
	cjson, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		return false
	}

	return cjson.ContainerJSONBase.State.Running
}

// healthCheckRunning is a pre-implemented HealthCheckFunc, which
// just checks if the docker container is up and running.
func (c *Container) healthCheckRunning() HealthCheckFunc {
	return func(ctx context.Context) error {
		if containerIsRunning(ctx, c.cli, c.ID) == false {
			return fmt.Errorf("container not running")
		}
		return nil
	}
}
