// A package that simplifies integration testing with docker.
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

// SuiteOpts is an option struct for getting or creating a suite in GetOrCreateSuite.
type SuiteOpts struct {
	// optional docker client, if one already exists
	Client *client.Client
	// whether to fail on instantiation errors
	Skip bool
}

// Suite represents a testing suite with a docker setup.
type Suite struct {
	name    string
	t       testing.TB
	cli     *client.Client
	network *Network
}

// GetOrCreateSuite returns a suite with the given name. If such suite is not registered yet it creates it.
// Returns true if the suite was already there, otherwise false.
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

// UnregisterAll unregisters all suites by closing the networks.
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

// Container creates a new docker container configuration with the given options.
func (s *Suite) Container(opts ContainerOpts) *Container {
	return NewContainer(s.t, s.cli, opts)
}

// Network creates a new docker network configuration with the given options.
func (s *Suite) Network(opts NetworkOpts) *Network {
	s.network = NewNetwork(s.t, s.cli, opts)
	return s.network
}

// Reset "resets" the underlying docker containers in the network. This
// calls the ResetFunc and HealthCheckFunc for each of them. These can be passed in
// ContainerOpts when creating a container.
//
// The context is passed explicitly to ResetFunc, where it can be used and
// implicitly to HealthCheckFunc where it may cancel the blocking health
// check loop.
func (s *Suite) Reset(ctx context.Context) {
	if s.network != nil {
		s.network.reset(ctx)
	}
}
