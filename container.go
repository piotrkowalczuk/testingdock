package testingdock

import (
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
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type HealthCheckFunc func() error

// HealthCheckHTTP ...
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

type ResetFunc func(ctx context.Context, cli *client.Client, c *Container) error

func ResetRestart() ResetFunc {
	return func(ctx context.Context, cli *client.Client, c *Container) error {
		return cli.ContainerRestart(ctx, c.ID, nil)
	}
}

func ResetCustom(fn func() error) ResetFunc {
	return func(ctx context.Context, cli *client.Client, c *Container) error {
		return fn()
	}
}

type ContainerOpts struct {
	ForcePull   bool
	Config      *container.Config
	HostConfig  *container.HostConfig
	Name        string
	HealthCheck HealthCheckFunc
	Reset       ResetFunc
}

type Container struct {
	t               testing.TB
	forcePull       bool
	cli             *client.Client
	network         *Network
	ccfg            *container.Config
	hcfg            *container.HostConfig
	ID, Name, Image string
	healthcheck     HealthCheckFunc
	children        []*Container
	cancel          func()
	reset           ResetFunc
}

func newContainer(t testing.TB, c *client.Client, opts ContainerOpts) *Container {
	return &Container{
		t:           t,
		forcePull:   opts.ForcePull,
		Name:        opts.Name,
		healthcheck: opts.HealthCheck,
		cli:         c,
		ccfg:        opts.Config,
		hcfg:        opts.HostConfig,
		reset:       opts.Reset,
	}
}

func (c *Container) Start(ctx context.Context) {
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

	containerListArgs := filters.NewArgs()
	containerListArgs.Add("name", c.Name)
	containers, err := c.cli.ContainerList(ctx, types.ContainerListOptions{
		Filters: containerListArgs,
	})
	if err != nil {
		c.t.Fatalf("container creation failure: %s", err.Error())
	}
	for _, cont := range containers {
		if err = c.cli.ContainerRemove(ctx, cont.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
			c.t.Fatalf("container removal failure: %s", err.Error())
		}
		printf("(setup ) %-25s (%s) - container removed", cont.Names[0], cont.ID)
	}

	cont, err := c.cli.ContainerCreate(ctx, c.ccfg, c.hcfg, nil, c.Name)
	if err != nil {
		c.t.Fatalf("container creation failure: %s", err.Error())
	}

	c.ID = cont.ID

	if err = c.cli.ContainerStart(ctx, c.ID, types.ContainerStartOptions{}); err != nil {
		c.t.Fatalf("container start failure: %s", err.Error())
	}

	c.cancel = func() {
		if err := c.cli.NetworkDisconnect(ctx, c.network.id, c.ID, true); err != nil {
			c.t.Fatalf("container disconnect failure: %s", err.Error())
		}
		printf("(cancel) %-25s (%s) - container disconnected from: %s", c.Name, c.ID, c.network.name)
		if err := c.cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
			c.t.Fatalf("container removal failure: %s", err.Error())
		}
		printf("(cancel) %-25s (%s) - container removed", c.Name, c.ID)
	}

	if err = c.cli.NetworkConnect(ctx, c.network.id, c.Name, &network.EndpointSettings{}); err != nil {
		c.cancel()
		c.t.Fatalf("container start failure: %s", err.Error())
	}
	printf("(setup ) %-25s (%s) - container connected to the network: %s", c.Name, c.ID, c.network.name)

	c.executeHealthCheck(ctx)

	for _, cont := range c.children {
		cont.Start(ctx)
	}
}

func (c *Container) Close() error {
	for _, cont := range c.children {
		cont.Close()
	}
	c.cancel()
	return nil
}

func (c *Container) After(cc *Container) {
	cc.network = c.network
	c.children = append(c.children, cc)
}

func (c *Container) Reset(ctx context.Context) {
	c.reset(ctx, c.cli, c)
	c.executeHealthCheck(ctx)

	for _, cc := range c.children {
		cc.Reset(ctx)
	}
}

func (c *Container) executeHealthCheck(ctx context.Context) {
	if c.healthcheck == nil {
		return
	}
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
