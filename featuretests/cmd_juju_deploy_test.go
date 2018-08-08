// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/juju/testcharms"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/juju/testing"
)

type cmdDeploySuite struct {
	testing.JujuConnSuite
}

func (s *cmdUpdateSeriesSuite) TestLocalDeploySuccess(c *gc.C) {
	ch := testcharms.Repo.CharmDir("storage-filesystem-subordinate") // has hooks

	ctx, err := runCommand(c, "deploy", ch.Path, "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `Deploying charm "local:quantal/storage-filesystem-subordinate-1"`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	savedCh, err := s.State.Charm(charm.MustParseURL("local:quantal/storage-filesystem-subordinate-1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCh, gc.NotNil)
}

func (s *cmdUpdateSeriesSuite) TestLocalDeployFailNoHook(c *gc.C) {
	ch := testcharms.Repo.CharmDir("riak") // has no hooks

	ctx, err := runCommand(c, "deploy", ch.Path, "--series", "quantal")
	c.Assert(err, gc.NotNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `invalid charm "riak": has no hooks`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	_, err = s.State.Charm(charm.MustParseURL("local:quantal/riak"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
