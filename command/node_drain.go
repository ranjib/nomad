package command

import (
	"fmt"
	"strings"
)

type NodeDrainCommand struct {
	Meta
}

func (c *NodeDrainCommand) Help() string {
	helpText := `
Usage: nomad node-drain [options] <node>

  Toggles node draining on a specified node. It is required
  that either -enable or -disable is specified, but not both.

General Options:

  ` + generalOptionsUsage() + `

Node Drain Options:

  -disable
    Disable draining for the specified node.

  -enable
    Enable draining for the specified node.
`
	return strings.TrimSpace(helpText)
}

func (c *NodeDrainCommand) Synopsis() string {
	return "Toggle drain mode on a given node"
}

func (c *NodeDrainCommand) Run(args []string) int {
	var enable, disable bool

	flags := c.Meta.FlagSet("node-drain", FlagSetClient)
	flags.Usage = func() { c.Ui.Output(c.Help()) }
	flags.BoolVar(&enable, "enable", false, "Enable drain mode")
	flags.BoolVar(&disable, "disable", false, "Disable drain mode")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	// Check that we got either enable or disable, but not both.
	if (enable && disable) || (!enable && !disable) {
		c.Ui.Error(c.Help())
		return 1
	}

	// Check that we got a node ID
	args = flags.Args()
	if len(args) != 1 {
		c.Ui.Error(c.Help())
		return 1
	}
	nodeID := args[0]

	// Get the HTTP client
	client, err := c.Meta.Client()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error initializing client: %s", err))
		return 1
	}

	// Check if node exists
	node, _, err := client.Nodes().Info(nodeID, nil)
	if err != nil {
		// Exact lookup failed, try with prefix based search
		nodes, _, err := client.Nodes().PrefixList(nodeID)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error toggling drain mode: %s", err))
			return 1
		}
		// Return error if no nodes are found
		if len(nodes) == 0 {
			c.Ui.Error(fmt.Sprintf("No node(s) with prefix or id %q found", nodeID))
			return 1
		}
		if len(nodes) > 1 {
			// Format the nodes list that matches the prefix so that the user
			// can create a more specific request
			out := make([]string, len(nodes)+1)
			out[0] = "ID|DC|Name|Class|Drain|Status"
			for i, node := range nodes {
				out[i+1] = fmt.Sprintf("%s|%s|%s|%s|%v|%s",
					node.ID,
					node.Datacenter,
					node.Name,
					node.NodeClass,
					node.Drain,
					node.Status)
			}
			// Dump the output
			c.Ui.Output(fmt.Sprintf("Prefix matched multiple nodes\n\n%s", formatList(out)))
			return 0
		}
		// Prefix lookup matched a single node
		node, _, err = client.Nodes().Info(nodes[0].ID, nil)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error toggling drain mode: %s", err))
			return 1
		}
	}

	// Toggle node draining
	if _, err := client.Nodes().ToggleDrain(node.ID, enable, nil); err != nil {
		c.Ui.Error(fmt.Sprintf("Error toggling drain mode: %s", err))
		return 1
	}
	return 0
}
