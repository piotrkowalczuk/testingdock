package testingdock

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

func init() {
	registry = make(map[string]*Suite)
}

var registry map[string]*Suite

type SuiteOpts struct {
	Client  *client.Client
	Skip    bool
	Timeout time.Duration
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
		if opts.Timeout != 0 {
			ctx, cancel := context.WithTimeout(context.TODO(), opts.Timeout)
			defer cancel()

			s.Reset(ctx)
		} else {
			s.Reset(context.Background())
		}
		return s
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
		//c.UpdateClientVersion(c.ClientVersion())
	}

	s := &Suite{cli: c, t: t, name: name}
	registry[s.name] = s
	return s
}

func UnregisterAll() {
	for name, reg := range registry {
		if reg.network == nil {
			continue
		}
		if err := reg.network.Close(); err != nil {
			printf("network (%s) close failure: %s", name, err.Error())
		} else {
			printf("network (%s) closed", name)
		}
		delete(registry, name)
	}
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
