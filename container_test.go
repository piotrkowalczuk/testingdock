package testingdock_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/lib/pq"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/piotrkowalczuk/testingdock"
)

func TestContainer_Start(t *testing.T) {
	name := "testingdock-test"
	s := testingdock.GetOrCreateSuite(t, name, testingdock.SuiteOpts{})
	n := s.Network(testingdock.NetworkOpts{
		Name: name,
	})

	postgresPort := testingdock.RandomPort(t)
	mnemosynePort := testingdock.RandomPort(t)
	mnemosyneDebugPort := testingdock.RandomPort(t)

	db, err := sql.Open("postgres", "postgres://postgres:@localhost:"+postgresPort+"?sslmode=disable")
	if err != nil {
		t.Fatalf("database connection error: %s", err.Error())
	}
	postgres := s.Container(testingdock.ContainerOpts{
		Name:      "postgres",
		ForcePull: true,
		Config: &container.Config{
			Image: "postgres:9.6",
		},
		HostConfig: &container.HostConfig{
			PortBindings: nat.PortMap{
				nat.Port("5432/tcp"): []nat.PortBinding{
					{
						HostPort: postgresPort,
					},
				},
			},
		},
		HealthCheck: db.Ping,
		Reset: testingdock.ResetCustom(func() error {
			_, err := db.Exec(`
				DROP SCHEMA public CASCADE;
				DROP SCHEMA mnemosyne CASCADE;
				CREATE SCHEMA public;
			`)
			return err
		}),
	})
	mnemosyned := s.Container(testingdock.ContainerOpts{
		Name:      "mnemosyned",
		ForcePull: true,
		Config: &container.Config{
			Image: "piotrkowalczuk/mnemosyne:v0.8.4",
		},
		HostConfig: &container.HostConfig{
			PortBindings: nat.PortMap{
				nat.Port("8080/tcp"): []nat.PortBinding{{HostPort: mnemosynePort}},
				nat.Port("8081/tcp"): []nat.PortBinding{{HostPort: mnemosyneDebugPort}},
			},
		},
		HealthCheck: testingdock.HealthCheckHTTP("http://localhost:" + mnemosyneDebugPort + "/health"),
		Reset:       testingdock.ResetRestart(),
	})

	n.After(postgres)
	postgres.After(mnemosyned)

	n.Start(context.TODO())
	defer n.Close()

	testQueries(t, db)

	s.Reset(context.TODO())

	testQueries(t, db)
}

func testQueries(t *testing.T, db *sql.DB) {
	_, err := db.ExecContext(context.TODO(), "CREATE TABLE public.example (name TEXT);")
	if err != nil {
		t.Fatalf("table creation error: %s", err.Error())
	}
	_, err = db.ExecContext(context.TODO(), "INSERT INTO public.example (name) VALUES ('anything')")
	if err != nil {
		t.Fatalf("insert error: %s", err.Error())
	}
	_, err = db.ExecContext(context.TODO(), "INSERT INTO mnemosyne.session (access_token, refresh_token,subject_id, bag) VALUES ('123', '123', '1', '{}')")
	if err != nil {
		t.Fatalf("insert error: %s", err.Error())
	}
}
