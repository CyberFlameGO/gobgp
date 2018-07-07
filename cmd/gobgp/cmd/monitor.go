// Copyright (C) 2015 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/spf13/cobra"

	api "github.com/osrg/gobgp/api"
	"github.com/osrg/gobgp/pkg/packet/bgp"
)

func makeMonitorRouteArgs(p *api.Path, showIdentifier bgp.BGPAddPathMode) []interface{} {
	pathStr := make([]interface{}, 0)

	// Title
	title := "ROUTE"
	if p.IsWithdraw {
		title = "DELROUTE"
	}
	pathStr = append(pathStr, title)

	// NLRI
	// If Add-Path required, append Path Identifier.
	nlri, _ := p.GetNativeNlri()
	if showIdentifier != bgp.BGP_ADD_PATH_NONE {
		pathStr = append(pathStr, p.GetIdentifier())
	}
	pathStr = append(pathStr, nlri)

	attrs, _ := p.GetNativePathAttributes()
	// Next Hop
	nexthop := "fictitious"
	if n := getNextHopFromPathAttributes(attrs); n != nil {
		nexthop = n.String()
	}
	pathStr = append(pathStr, nexthop)

	// AS_PATH
	aspathstr := func() string {
		for _, attr := range attrs {
			switch a := attr.(type) {
			case *bgp.PathAttributeAsPath:
				return bgp.AsPathString(a)
			}
		}
		return ""
	}()
	pathStr = append(pathStr, aspathstr)

	// Path Attributes
	pathStr = append(pathStr, getPathAttributeString(nlri, attrs))

	return pathStr
}

func monitorRoute(pathList []*api.Path, showIdentifier bgp.BGPAddPathMode) {
	var pathStrs [][]interface{}

	for _, p := range pathList {
		pathStrs = append(pathStrs, makeMonitorRouteArgs(p, showIdentifier))
	}

	format := "[%s] %s via %s aspath [%s] attrs %s\n"
	if showIdentifier != bgp.BGP_ADD_PATH_NONE {
		format = "[%s] %d:%s via %s aspath [%s] attrs %s\n"
	}
	for _, pathStr := range pathStrs {
		fmt.Printf(format, pathStr...)
	}
}

func NewMonitorCmd() *cobra.Command {

	var current bool

	monitor := func(recver interface {
		Recv() (*api.Destination, error)
	}, showIdentifier bgp.BGPAddPathMode) {
		for {
			dst, err := recver.Recv()
			if err == io.EOF {
				break
			} else if err != nil {
				exitWithError(err)
			}
			if globalOpts.Json {
				j, _ := json.Marshal(dst.Paths)
				fmt.Println(string(j))
			} else {
				monitorRoute(dst.Paths, showIdentifier)
			}
		}
	}

	ribCmd := &cobra.Command{
		Use: CMD_RIB,
		Run: func(cmd *cobra.Command, args []string) {
			family, err := checkAddressFamily(bgp.RouteFamily(0))
			if err != nil {
				exitWithError(err)
			}
			recver, err := client.MonitorRIB(family, current)
			if err != nil {
				exitWithError(err)
			}
			monitor(recver, bgp.BGP_ADD_PATH_NONE)
		},
	}
	ribCmd.PersistentFlags().StringVarP(&subOpts.AddressFamily, "address-family", "a", "", "address family")

	globalCmd := &cobra.Command{
		Use: CMD_GLOBAL,
	}
	globalCmd.AddCommand(ribCmd)

	neighborCmd := &cobra.Command{
		Use:  fmt.Sprintf("%s [<neighbor address>]", CMD_NEIGHBOR),
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			stream, err := client.MonitorNeighborState(name, current)
			if err != nil {
				exitWithError(err)
			}
			for {
				s, err := stream.Recv()
				if err == io.EOF {
					break
				} else if err != nil {
					exitWithError(err)
				}
				if globalOpts.Json {
					j, _ := json.Marshal(s)
					fmt.Println(string(j))
				} else {
					addr := s.State.NeighborAddress
					if s.Config.NeighborInterface != "" {
						addr = fmt.Sprintf("%s(%s)", addr, s.Config.NeighborInterface)
					}
					fmt.Printf("[NEIGH] %s fsm: %s admin: %s\n", addr, s.State.SessionState, s.State.AdminState)
				}
			}
		},
	}

	adjInCmd := &cobra.Command{
		Use: CMD_ADJ_IN,
		Run: func(cmd *cobra.Command, args []string) {
			name := ""
			if len(args) > 0 {
				remoteIP := net.ParseIP(args[0])
				if remoteIP == nil {
					exitWithError(fmt.Errorf("invalid ip address: %s", args[0]))
				}
				name = args[0]
			}
			family, err := checkAddressFamily(bgp.RouteFamily(0))
			if err != nil {
				exitWithError(err)
			}
			recver, err := client.MonitorAdjRIBIn(name, family, current)
			if err != nil {
				exitWithError(err)
			}
			monitor(recver, bgp.BGP_ADD_PATH_RECEIVE)
		},
	}
	adjInCmd.PersistentFlags().StringVarP(&subOpts.AddressFamily, "address-family", "a", "", "address family")

	monitorCmd := &cobra.Command{
		Use: CMD_MONITOR,
	}
	monitorCmd.AddCommand(globalCmd)
	monitorCmd.AddCommand(neighborCmd)
	monitorCmd.AddCommand(adjInCmd)

	monitorCmd.PersistentFlags().BoolVarP(&current, "current", "", false, "dump current contents")

	return monitorCmd
}