// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
)

type RelationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&RelationSuite{})

func (s *RelationSuite) TestAddRelationErrors(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	riak := s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)

	// Check we can't add a relation with services that don't exist.
	yoursqlEP := mysqlEP
	yoursqlEP.ApplicationName = "yoursql"
	_, err = s.State.AddRelation(yoursqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db yoursql:server": application "yoursql" does not exist`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that interfaces have to match.
	msep3 := mysqlEP
	msep3.Interface = "roflcopter"
	_, err = s.State.AddRelation(msep3, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": endpoints do not relate`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check a variety of surprising endpoint combinations.
	_, err = s.State.AddRelation(wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db": relation must have two endpoints`)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db riak:ring": endpoints do not relate`)
	assertOneRelation(c, riak, 0, riakEP)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, riakEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "riak:ring riak:ring": endpoints do not relate`)
	assertOneRelation(c, riak, 0, riakEP)

	_, err = s.State.AddRelation()
	c.Assert(err, gc.ErrorMatches, `cannot add relation "": relation must have two endpoints`)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP, riakEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server riak:ring": relation must have two endpoints`)
	assertOneRelation(c, riak, 0, riakEP)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that a relation can't be added to a Dying service.
	_, err = wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": application "wordpress" is not alive`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)
}

func (s *RelationSuite) TestRetrieveSuccess(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	expect, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.EndpointsRelation(wordpressEP, mysqlEP)
	check := func() {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rel.Id(), gc.Equals, expect.Id())
		c.Assert(rel.String(), gc.Equals, expect.String())
	}
	check()
	rel, err = s.State.EndpointsRelation(mysqlEP, wordpressEP)
	check()
	rel, err = s.State.Relation(expect.Id())
	check()
}

func (s *RelationSuite) TestRetrieveNotFound(c *gc.C) {
	subway := state.Endpoint{
		ApplicationName: "subway",
		Relation: charm.Relation{
			Name:      "db",
			Interface: "mongodb",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	mongo := state.Endpoint{
		ApplicationName: "mongo",
		Relation: charm.Relation{
			Name:      "server",
			Interface: "mongodb",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.EndpointsRelation(subway, mongo)
	c.Assert(err, gc.ErrorMatches, `relation "subway:db mongo:server" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.State.Relation(999)
	c.Assert(err, gc.ErrorMatches, `relation 999 not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RelationSuite) TestAddRelation(c *gc.C) {
	// Add a relation.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)

	// Check we cannot re-add the same relation, regardless of endpoint ordering.
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server already exists`)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server already exists`)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestAddRelationSeriesNeedNotMatch(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddSeriesCharm(c, "mysql", "otherseries"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestAddContainerRelation(c *gc.C) {
	// Add a relation.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the endpoints both have container scope.
	wordpressEP.Scope = charm.ScopeContainer
	assertOneRelation(c, logging, 0, loggingEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, loggingEP)

	// Check we cannot re-add the same relation, regardless of endpoint ordering.
	_, err = s.State.AddRelation(loggingEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": relation logging:info wordpress:juju-info already exists`)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": relation logging:info wordpress:juju-info already exists`)
	assertOneRelation(c, logging, 0, loggingEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, loggingEP)
}

func (s *RelationSuite) TestAddContainerRelationSeriesMustMatch(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging := s.AddTestingService(c, "logging", s.AddSeriesCharm(c, "logging", "otherseries"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": principal and subordinate applications' series must match`)
}

func (s *RelationSuite) TestAddContainerRelationWithNoSubordinate(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressSubEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wordpressSubEP.Scope = charm.ScopeContainer
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRelation(mysqlEP, wordpressSubEP)
	c.Assert(err, gc.ErrorMatches,
		`cannot add relation "wordpress:db mysql:server": container scoped relation requires at least one subordinate application`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)
}

func (s *RelationSuite) TestAddContainerRelationWithTwoSubordinates(c *gc.C) {
	loggingCharm := s.AddTestingCharm(c, "logging")
	logging1 := s.AddTestingService(c, "logging1", loggingCharm)
	logging1EP, err := logging1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging2 := s.AddTestingService(c, "logging2", loggingCharm)
	logging2EP, err := logging2.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRelation(logging1EP, logging2EP)
	c.Assert(err, jc.ErrorIsNil)
	// AddRelation changes the scope on the endpoint if relation is container scoped.
	logging1EP.Scope = charm.ScopeContainer
	assertOneRelation(c, logging1, 0, logging1EP, logging2EP)
	assertOneRelation(c, logging2, 0, logging2EP, logging1EP)
}

func (s *RelationSuite) TestDestroyRelation(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Test that the relation can be destroyed.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that a second destroy is a no-op.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Create a new relation and check that refreshing the old does not find
	// the new.
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RelationSuite) TestDestroyPeerRelation(c *gc.C) {
	// Check that a peer relation cannot be destroyed directly.
	riakch := s.AddTestingCharm(c, "riak")
	riak := s.AddTestingService(c, "riak", riakch)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel := assertOneRelation(c, riak, 0, riakEP)
	err = rel.Destroy()
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
	assertOneRelation(c, riak, 0, riakEP)

	// Check that it is destroyed when the service is destroyed.
	err = riak.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNoRelations(c, riak)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Create a new service (and hence a new relation in the background); check
	// that refreshing the old one does not accidentally get the new one.
	newriak := s.AddTestingService(c, "riak", riakch)
	assertOneRelation(c, newriak, 1, riakEP)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertNoRelations(c *gc.C, srv *state.Application) {
	rels, err := srv.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 0)
}

func assertOneRelation(c *gc.C, srv *state.Application, relId int, endpoints ...state.Endpoint) *state.Relation {
	rels, err := srv.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)

	rel := rels[0]
	c.Assert(rel.Id(), gc.Equals, relId)

	c.Assert(rel.Endpoints(), jc.SameContents, endpoints)

	name := srv.Name()
	expectEp := endpoints[0]
	ep, err := rel.Endpoint(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ep, gc.DeepEquals, expectEp)
	if len(endpoints) == 2 {
		expectEp = endpoints[1]
	}
	eps, err := rel.RelatedEndpoints(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{expectEp})
	return rel
}
