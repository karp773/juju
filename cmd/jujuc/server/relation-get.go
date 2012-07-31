package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	*HookContext
	RelationId int
	UnitName   string
	Key        string
	out        cmd.Output
	testMode   bool
}

func NewRelationGetCommand(ctx *HookContext) (cmd.Command, error) {
	return &RelationGetCommand{HookContext: ctx}, nil
}

func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit>"
	if c.RemoteUnitName != "" {
		args = fmt.Sprintf("[<key> [<unit (= %q)]]", c.RemoteUnitName)
	}
	return &cmd.Info{
		"relation-get", args, "get relation settings", `
Specifying a key will cause a single settings value to be returned. Leaving
key empty, or setting it to "-", will cause all keys and values to be returned.
`,
	}
}

func (c *RelationGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	// TODO FWER implement --format shell
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
	f.BoolVar(&c.testMode, "test", false, "returns non-zero exit code if value is false/zero/empty")
	relationId, err := c.parseRelationId(f, args)
	if err != nil {
		return err
	}
	c.RelationId = relationId
	args = f.Args()
	c.Key = ""
	if len(args) > 0 {
		if c.Key = args[0]; c.Key == "-" {
			c.Key = ""
		}
		args = args[1:]
	}
	c.UnitName = c.RemoteUnitName
	if len(args) > 0 {
		c.UnitName = args[1]
		args = args[1:]
	}
	if c.UnitName == "" {
		return fmt.Errorf("unit not specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationGetCommand) Run(ctx *cmd.Context) error {
	var settings map[string]interface{}
	if c.UnitName == c.Unit.Name() {
		node, err := c.Relations[c.RelationId].Settings()
		if err != nil {
			return err
		}
		settings = node.Map()
	} else {
		var err error
		settings, err = c.Relations[c.RelationId].ReadSettings(c.UnitName)
		if err != nil {
			return err
		}
	}
	var value interface{}
	if c.Key == "" {
		value = settings
	} else {
		value, _ = settings[c.Key]
	}
	if c.testMode {
		return truthError(value)
	}
	return c.out.Write(ctx, value)
}
