// Copyright (c) 2016 Tigera, Inc. All rights reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package commands

import (
	"os"

	"github.com/docopt/docopt-go"

	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/projectcalico/calicoctl/calicoctl/commands/constants"
)

func Get(args []string) {
	doc := constants.DatastoreIntro + `Usage:
  calicoctl get ([--scope=<SCOPE>] [--node=<NODE>] [--orchestrator=<ORCH>]
                 [--workload=<WORKLOAD>] (<KIND> [<NAME>]) |
                --filename=<FILENAME>)
                [--output=<OUTPUT>] [--config=<CONFIG>]

Examples:
  # List all policy in default output format.
  calicoctl get policy

  # List a specific policy in YAML format
  calicoctl get -o yaml policy my-policy-1

Options:
  -h --help                    Show this screen.
  -f --filename=<FILENAME>     Filename to use to get the resource.  If set to
                               "-" loads from stdin.
  -o --output=<OUTPUT FORMAT>  Output format.  One of: yaml, json, ps, wide,
                               custom-columns=..., go-template=...,
                               go-template-file=...   [Default: ps]
  -n --node=<NODE>             The node (this may be the hostname of the
                               compute server if your installation does not
                               explicitly set the names of each Calico node).
     --orchestrator=<ORCH>     The orchestrator (valid for workload endpoints).
     --workload=<WORKLOAD>     The workload (valid for workload endpoints).
     --scope=<SCOPE>           The scope of the resource type.  One of global,
                               node.  This is only valid for BGP peers and is
                               used to indicate whether the peer is a global
                               peer or node-specific.
  -c --config=<CONFIG>         Path to the file containing connection
                               configuration in YAML or JSON format.
                               [default: /etc/calico/calicoctl.cfg]

Description:
  The get command is used to display a set of resources by filename or stdin,
  or by type and identifiers.  JSON and YAML formats are accepted for file and
  stdin format.

  Valid resource types are:

    * node
    * bgpPeer
    * hostEndpoint
    * workloadEndpoint
    * ipPool
    * policy
    * profile

  The resource type is case insensitive and may be pluralized.

  Attempting to get resources that do not exist will simply return no results.

  When getting resources by type, only a single type may be specified at a
  time.  The name and other identifiers (hostname, scope) are optional, and are
  wildcarded when omitted. Thus if you specify no identifiers at all (other
  than type), then all configured resources of the requested type will be
  returned.

  By default the results are output in a ps-style table output.  There are
  alternative ways to display the data using the --output option:

    ps                    Display the results in ps-style output.
    wide                  As per the ps option, but includes more headings.
    custom-columns        As per the ps option, but only display the columns
                          that are requested in the comma-separated list.
    golang-template       Display the results using the specified golang
                          template.  This can be used to filter results, for
                          example to return a specific value.
    golang-template-file  Display the results using the golang template that is
                          contained in the specified file.
    yaml                  Display the results in YAML output format.
    json                  Display the results in JSON output format.

  Note that the data output using YAML or JSON format is always valid to use as
  input to all of the resource management commands (create, apply, replace,
  delete, get).

  Please refer to the docs at http://docs.projectcalico.org for more details on
  the output formats, including example outputs, resource structure (required
  for the golang template definitions) and the valid column names (required for
  the custom-columns option).
`
	parsedArgs, err := docopt.Parse(doc, args, true, "", false, false)
	if err != nil {
		fmt.Printf("Invalid option: 'calicoctl %s'. Use flag '--help' to read about a specific subcommand.\n", strings.Join(args, " "))
		os.Exit(1)
	}
	if len(parsedArgs) == 0 {
		return
	}

	var rp resourcePrinter
	output := parsedArgs["--output"].(string)
	switch output {
	case "yaml":
		rp = resourcePrinterYAML{}
	case "json":
		rp = resourcePrinterJSON{}
	case "ps":
		rp = resourcePrinterTable{wide: false}
	case "wide":
		rp = resourcePrinterTable{wide: true}
	default:
		// Output format may be a key=value pair, so split on "=" to find out.  Pull
		// out the key and value, and split the value by "," as some options allow
		// a multiple-valued value.
		outputParms := strings.SplitN(output, "=", 2)
		outputKey := outputParms[0]
		outputValue := ""
		outputValues := []string{}
		if len(outputParms) == 2 {
			outputValue = outputParms[1]
			outputValues = strings.Split(outputValue, ",")
		}

		switch outputKey {
		case "go-template":
			if outputValue == "" {
				fmt.Printf("need to specify a template")
				os.Exit(1)
			}
			rp = resourcePrinterTemplate{template: outputValue}
		case "go-template-file":
			if outputValue == "" {
				fmt.Printf("need to specify a template file")
				os.Exit(1)
			}
			rp = resourcePrinterTemplateFile{templateFile: outputValue}
		case "custom-columns":
			if outputValue == "" {
				fmt.Printf("need to specify at least one column")
				os.Exit(1)
			}
			rp = resourcePrinterTable{headings: outputValues}
		}
	}

	if rp == nil {
		fmt.Printf("unrecognized output format '%s'", output)
		os.Exit(1)
	}

	results := executeConfigCommand(parsedArgs, actionList)
	log.Infof("results: %+v", results)

	if results.fileInvalid {
		fmt.Printf("Failed to execute command: %v\n", results.err)
		os.Exit(1)
	} else if results.err != nil {
		fmt.Printf("Failed to get resources: %v\n", results.err)
		os.Exit(1)
	}

	err = rp.print(results.client, results.resources)
	if err != nil {
		fmt.Println(err)
	}
}
