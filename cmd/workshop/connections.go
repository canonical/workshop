package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/canonical/x-go/i18n"
	"github.com/spf13/cobra"

	"github.com/canonical/workshop/client"
)

type CmdConnections struct {
	root *CmdRoot
	all  bool
}

func (c *CmdConnections) Command() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "connections [<WORKSHOP>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "List interface connections",
		Long: `
This command lists the connections between interface plugs and slots
for the entire project or a single workshop within it.
Each line represents a connection between a plug and a slot via an interface;
additional notes, including specific plug bindings, are provided as needed.


Notes:

- The output lists connections created with 'workshop connect' as 'manual'

- The '--all' option needn't be used with an argument;
  if a workshop is supplied, disconnected plugs are also listed
`,
		RunE: c.Run,
	}

	cmd.Flags().BoolVar(&c.all, "all", false, "Include disconnected plugs in the output.")

	return cmd
}

func isSystemSdk(sdkName string) bool {
	return sdkName == "system"
}

func isDefaultSystemSlot(slot client.Slot) bool {
	return isSystemSdk(slot.Sdk) && slot.Interface == slot.Name
}

func endpoint(workshop, sdkName, name string) string {
	if isSystemSdk(sdkName) {
		return ":" + name
	}
	return workshop + "/" + sdkName + ":" + name
}

type connection struct {
	slot          string
	plug          string
	interfaceName string
	manual        bool
	bind          string
	bindIdx       int
}

func (cn connection) String() string {
	opts := []string{}
	if cn.manual {
		opts = append(opts, "manual")
	}
	if cn.bind != "" && cn.bindIdx > 0 {
		opts = append(opts, fmt.Sprintf("bind.%d", cn.bindIdx))
	}
	if len(opts) == 0 {
		return "-"
	}
	return strings.Join(opts, ",")
}

type byConnectionData []connection

func (b byConnectionData) Len() int      { return len(b) }
func (b byConnectionData) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byConnectionData) Less(i, j int) bool {
	iCon, jCon := b[i], b[j]
	if iCon.interfaceName != jCon.interfaceName {
		return iCon.interfaceName < jCon.interfaceName
	}
	if iCon.plug != jCon.plug {
		return iCon.plug < jCon.plug
	}
	return iCon.slot < jCon.slot
}

func maybeBound(plug client.PlugRef, plugs []client.Plug) string {
	var bind string

	// check if this plug is bound to
	idx := slices.IndexFunc(plugs, func(p client.Plug) bool {
		return p.Workshop == plug.Workshop && p.Sdk == plug.Sdk && p.Name == plug.Name
	})
	if idx != -1 {
		info := plugs[idx]
		if info.Bind != nil {
			bind = endpoint(info.Bind.Workshop, info.Bind.Sdk, info.Bind.Name)
			return bind
		}
	}

	// check if other plugs are bound to this one
	idx = slices.IndexFunc(plugs, func(p client.Plug) bool {
		if p.Bind != nil {
			return *p.Bind == plug
		}
		return false
	})
	if idx != -1 {
		bind = endpoint(plug.Workshop, plug.Sdk, plug.Name)
		return bind
	}

	// not bound or bound to
	return ""
}

func (c *CmdConnections) Run(cmd *cobra.Command, av []string) error {
	cli, err := c.root.client()
	if err != nil {
		return err
	}

	project, err := cli.Project(c.root.project)
	if err != nil {
		return err
	}

	workshop := ""
	if len(av) > 0 {
		workshop = av[0]
		if c.all {
			// passing a workshop name already implies --all, error out
			// when it was passed explicitly
			return fmt.Errorf("cannot use --all with workshop name")
		}
		c.all = true
	}

	connections, err := cli.Connections(&client.ConnectionOptions{ProjectId: project.Id, Workshop: workshop, All: c.all})
	if err != nil {
		return err
	}

	if len(connections.Plugs) == 0 && len(connections.Slots) == 0 {
		return nil
	}

	annotatedConns := make([]connection, 0, len(connections.Established)+len(connections.Undesired))
	for _, conn := range connections.Established {
		annotatedConns = append(annotatedConns, connection{
			plug:          endpoint(conn.Plug.Workshop, conn.Plug.Sdk, conn.Plug.Name),
			slot:          endpoint(conn.Slot.Workshop, conn.Slot.Sdk, conn.Slot.Name),
			manual:        conn.Manual,
			interfaceName: conn.Interface,
			bind:          maybeBound(conn.Plug, connections.Plugs),
		})
	}

	w := tabWriter()
	fmt.Fprintln(w, i18n.G("Interface\tPlug\tSlot\tNotes"))

	for _, plug := range connections.Plugs {
		if len(plug.Connections) == 0 && c.all {
			var bind = maybeBound(plug.Ref(), connections.Plugs)
			annotatedConns = append(annotatedConns, connection{
				plug:          endpoint(plug.Workshop, plug.Sdk, plug.Name),
				slot:          "-",
				interfaceName: plug.Interface,
				bind:          bind,
			})
		}
	}
	for _, slot := range connections.Slots {
		if isDefaultSystemSlot(slot) {
			// displaying unconnected system snap slots is boring,
			// unless explicitly asked to show them
			continue
		}
		if len(slot.Connections) == 0 && c.all {
			annotatedConns = append(annotatedConns, connection{
				plug:          "-",
				slot:          endpoint(slot.Workshop, slot.Sdk, slot.Name),
				interfaceName: slot.Interface,
			})
		}
	}

	sort.Sort(byConnectionData(annotatedConns))

	for _, note := range annotatedConns {
		// find the plug that the current connection is bound to
		idx := slices.IndexFunc(annotatedConns, func(c connection) bool { return c.plug != "" && note.bind != "" && c.plug == note.bind })
		note.bindIdx = idx + 1
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", note.interfaceName, note.plug, note.slot, note)
	}

	if len(annotatedConns) > 0 {
		w.Flush()
	}

	return nil
}
