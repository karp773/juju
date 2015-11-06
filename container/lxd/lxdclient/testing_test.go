// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"os"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	Stub   *gitjujutesting.Stub
	Client *stubClient
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Stub = &gitjujutesting.Stub{}
	s.Client = &stubClient{stub: s.Stub}
}

type stubClient struct {
	stub *gitjujutesting.Stub

	Instance   *shared.ContainerState
	Instances  []shared.ContainerInfo
	ReturnCode int
	Response   *lxd.Response
}

func (s *stubClient) WaitForSuccess(waitURL string) error {
	s.stub.AddCall("WaitForSuccess", waitURL)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) ContainerStatus(name string) (*shared.ContainerState, error) {
	s.stub.AddCall("ContainerStatus", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Instance, nil
}

func (s *stubClient) ListContainers() ([]shared.ContainerInfo, error) {
	s.stub.AddCall("ListContainers")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Instances, nil
}

func (s *stubClient) Init(name, remote, image string, profiles *[]string, ephem bool) (*lxd.Response, error) {
	s.stub.AddCall("AddInstance", name, remote, image, profiles, ephem)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Delete(name string) (*lxd.Response, error) {
	s.stub.AddCall("Delete", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Action(name string, action shared.ContainerAction, timeout int, force bool) (*lxd.Response, error) {
	s.stub.AddCall("Action", name, action, timeout, force)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Exec(name string, cmd []string, env map[string]string, stdin *os.File, stdout *os.File, stderr *os.File) (int, error) {
	s.stub.AddCall("Exec", name, cmd, env, stdin, stdout, stderr)
	if err := s.stub.NextErr(); err != nil {
		return -1, errors.Trace(err)
	}

	return s.ReturnCode, nil
}

func (s *stubClient) SetContainerConfig(name, key, value string) error {
	s.stub.AddCall("SetContainerConfig", name, key, value)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) ContainerDeviceAdd(name, devname, devtype string, props []string) (*lxd.Response, error) {
	s.stub.AddCall("ContainerDeviceAdd", name, devname, devtype, props)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}
