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
	Skip   bool
}

type Suite struct {
	name    string
	t       testing.TB
	cli     *client.Client
	network *Network
}

// GetOrCreateSuite returns suite with given name. If such suite is not registered yet it creates it.
// Returns true if suite was already there, otherwise false.
func GetOrCreateSuite(t testing.TB, name string, opts SuiteOpts) (*Suite, bool) {
	if s, ok := registry[name]; ok {
		return s, true
	}

	c := opts.Client
	if c == nil {
		var err error
		c, err = client.NewEnvClient()
		if err != nil {
			if opts.Skip {
				t.Skipf("docker client instantiation failure: %s", err.Error())
			} else {
				t.Fatalf("docker client instantiation failure: %s", err.Error())
			}
		}
	}

	s := &Suite{cli: c, t: t, name: name}
	registry[s.name] = s
	return s, false
}

func UnregisterAll() {
	printf("(unregi) start")
	for name, reg := range registry {
		if reg.network == nil {
			continue
		}
		if err := reg.network.Close(); err != nil {
			printf("(unregi) %-25s (%-64s) - suite unregister failure: %s", name, "", err.Error())
		} else {
			printf("(unregi) %-25s (%-64s) - suite unregistered", name, "")
		}
		delete(registry, name)
	}
	printf("(unregi) finished")
}

func (s *Suite) Container(opts ContainerOpts) *Container {
	return newContainer(s.t, s.cli, opts)
}

func (s *Suite) Network(opts NetworkOpts) *Network {
	s.network = newNetwork(s.t, s.cli, opts)
	return s.network
}

func (s *Suite) Reset(ctx context.Context) {
	if s.network != nil {
		s.network.reset(ctx)
	}
}
