package testingdock

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// HealthCheckFunc is the type of a health checking function, which is supposed
// to return nil on success, indicating that a container is not only "up", but
// "accessible" in the specified way.
//
// If the function returns an error, it will be called until it doesn't (blocking).
type HealthCheckFunc func() error

// HealthCheckHTTP is a pre-implemented HealthCheckFunc which checks if the given
// url returns http.StatusOk.
func HealthCheckHTTP(url string) HealthCheckFunc {
	return func() error {
		res, err := http.Get(url)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("wrong status code: %s", http.StatusText(res.StatusCode))
		}
		return nil
	}
}

// ResetFunc is the type of the container reset function, which is called on
// c.Reset().
type ResetFunc func(ctx context.Context, cli *client.Client, c *Container) error

// ResetRestart is a pre-implemented ResetFunc, which just restarts the container.
func ResetRestart() ResetFunc {
	return func(ctx context.Context, cli *client.Client, c *Container) error {
		return cli.ContainerRestart(ctx, c.ID, nil)
	}
}

// ResetCustom is just a convenience wrapper to set a ResetFunc.
func ResetCustom(fn func() error) ResetFunc {
	return func(ctx context.Context, cli *client.Client, c *Container) error {
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
	// is 'really' up, it will block until it returns nil.
	HealthCheck HealthCheckFunc
	// default is 30s
	HealthCheckTimeout time.Duration
	// Function called when the containers are reset
	Reset ResetFunc
	// shows container stdout/stderr
	Verbose bool
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
	verbose  bool
}

// Creates a new container configuration with the given options.
func newContainer(t testing.TB, c *client.Client, opts ContainerOpts) *Container {
	// set default
	if opts.HealthCheckTimeout == 0 { // zero value
		opts.HealthCheckTimeout = 30 * time.Second
	}

	// always autoremove
	opts.HostConfig.AutoRemove = true

	// set testingdock label
	opts.Config.Labels = createTestingLabel()

	return &Container{
		t:                  t,
		forcePull:          opts.ForcePull,
		Name:               opts.Name,
		healthcheck:        opts.HealthCheck,
		healthchecktimeout: opts.HealthCheckTimeout,
		cli:                c,
		ccfg:               opts.Config,
		hcfg:               opts.HostConfig,
		resetF:             opts.Reset,
		verbose:            opts.Verbose,
	}
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
		img, err := c.cli.ImagePull(ctx, c.ccfg.Image, types.ImagePullOptions{})
		if err != nil {
			c.t.Fatalf("image downloading failure: %s", err.Error())
		}
		if _, err = io.Copy(ioutil.Discard, img); err != nil {
			c.t.Fatalf("image pull response read failure: %s", err.Error())
		}
		if err = img.Close(); err != nil {
			c.t.Fatalf("image closing failure: %s", err.Error())
		}
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
	if c.verbose {
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
	for _, cont := range c.children {
		cont.start(ctx)
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
	for _, cont := range c.children {
		cont.close() // nolint: errcheck
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
	if err := c.resetF(ctx, c.cli, c); err != nil {
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
	if c.healthcheck == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, c.healthchecktimeout)
	defer cancel()
InfLoop:
	for {
		select {
		case <-ctx.Done():
			c.t.Fatalf("health check failure: %s", ctx.Err())
		case <-time.After(1 * time.Second):
			if err := c.healthcheck(); err != nil {
				printf("(setup ) %-25s (%s) - container health failure: %s", c.Name, c.ID, err.Error())
				continue InfLoop
			}
			break InfLoop
		}
	}
}
