package testingdock

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type NetworkOpts struct {
	Name string
}

// Network ...
type Network struct {
	t        testing.TB
	cli      *client.Client
	id, name string
	gateway  string
	cancel   func()
	children []*Container
}

func newNetwork(t testing.TB, c *client.Client, opts NetworkOpts) *Network {
	return &Network{
		t:    t,
		cli:  c,
		name: opts.Name,
	}
}

func (n *Network) Start(ctx context.Context) {
	n.initialCleanup(ctx)

	res, err := n.cli.NetworkCreate(ctx, n.name, types.NetworkCreate{})
	if err != nil {
		n.t.Fatalf("network creation failure: %s", err.Error())
	}
	n.id = res.ID
	n.cancel = func() {
		if err := n.cli.NetworkRemove(ctx, n.id); err != nil {
			n.t.Fatalf("network removal failure: %s", err.Error())
		}
		printf("(cancel) %-25s (%s) - network removed", n.name, n.id)
	}
	printf("(setup ) %-25s (%s) - network created", n.name, n.id)

	ni, err := n.cli.NetworkInspect(ctx, n.id, false)
	if err != nil {
		n.cancel()
		n.t.Fatalf("network inspect failure: %s", err.Error())
	}
	n.gateway = ni.IPAM.Config[0].Gateway
	printf("(setup ) %-25s (%s) - network got gateway ip: %s", n.name, n.id, n.gateway)
	for _, cont := range n.children {
		cont.Start(ctx)
	}
}

func (n *Network) initialCleanup(ctx context.Context) {
	networkListArgs := filters.NewArgs()
	networkListArgs.Add("name", n.name)

	networks, err := n.cli.NetworkList(ctx, types.NetworkListOptions{Filters: networkListArgs})
	if err != nil {
		n.t.Fatalf("network listing failure: %s", err.Error())
	}
	for _, nn := range networks {
		containers, err := n.cli.ContainerList(ctx, types.ContainerListOptions{All: true})
		if err != nil {
			n.t.Fatalf("container list failure: %s", err.Error())
		}
		for _, cc := range containers {
			for _, nnn := range cc.NetworkSettings.Networks {
				if nnn.NetworkID == nn.ID {
					if err = n.cli.ContainerRemove(ctx, cc.ID, types.ContainerRemoveOptions{
						RemoveVolumes: true,
						Force:         true,
					}); err != nil {
						n.t.Fatalf("container removal failure: %s", err.Error())
					}
					printf("(setup ) %-25s (%s) - network endpoint removed: %s", nn.Name, nn.ID, cc.Names[0])
				}
			}
		}
		if err = n.cli.NetworkRemove(ctx, nn.ID); err != nil {
			n.t.Fatalf("network removal failure: %s", err.Error())
		}
		printf("(setup ) %-25s (%s) - network removed", nn.Name, nn.ID)
	}
}

// Close implements io.Closer interface.
func (n *Network) Close() error {
	for _, cont := range n.children {
		cont.Close()
	}
	n.cancel()
	return nil
}

func (n *Network) After(c *Container) {
	c.network = n
	n.children = append(n.children, c)
}

func (n *Network) reset(ctx context.Context) {
	for _, c := range n.children {
		c.Reset(ctx)
	}
}
