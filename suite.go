package testingdock

import (
	"context"
	"testing"

	"github.com/docker/docker/client"
)

func init() {
	registry = make(map[string]*Suite)
}

var registry map[string]*Suite

type SuiteOpts struct {
	Client *client.Client
}

type Suite struct {
	name    string
	t       testing.TB
	cli     *client.Client
	network *Network
}

// GetOrCreateSuite ...
func GetOrCreateSuite(t testing.TB, name string, opts SuiteOpts) *Suite {
	if s, ok := registry[name]; ok {
		return s
	}

	c := opts.Client
	if c == nil {
		var err error
		c, err = client.NewEnvClient()
		if err != nil {
			t.Fatalf("docker client instantiation failure: %s", err.Error())
		}
	}

	s := &Suite{cli: c, t: t, name: name}
	registry[s.name] = s
	return s
}

func (s *Suite) Container(opts ContainerOpts) *Container {
	return newContainer(s.t, s.cli, opts)
}

func (s *Suite) Network(opts NetworkOpts) *Network {
	s.network = newNetwork(s.t, s.cli, opts)
	return s.network
}

func (s *Suite) Reset(ctx context.Context) {
	s.network.reset(ctx)
}
