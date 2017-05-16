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
	"fmt"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docopt/docopt-go"

	"github.com/projectcalico/calicoctl/calicoctl/commands/constants"
)

func Apply(args []string) {
	doc := constants.DatastoreIntro + `Usage:
  calicoctl apply --filename=<FILENAME> [--config=<CONFIG>]

Examples:
  # Apply a policy using the data in policy.yaml.
  calicoctl apply -f ./policy.yaml

  # Apply a policy based on the JSON passed into stdin.
  cat policy.json | calicoctl apply -f -

Options:
  -h --help                 Show this screen.
  -f --filename=<FILENAME>  Filename to use to apply the resource.  If set to
                            "-" loads from stdin.
  -c --config=<CONFIG>      Path to the file containing connection
                            configuration in YAML or JSON format.
                            [default: /etc/calico/calicoctl.cfg]

Description:
  The apply command is used to create or replace a set of resources by filename
  or stdin.  JSON and YAML formats are accepted.

  Valid resource types are:

    * node
    * bgpPeer
    * hostEndpoint
    * workloadEndpoint
    * ipPool
    * policy
    * profile

  When applying a resource:
  -  if the resource does not already exist (as determined by it's primary
     identifiers) then it is created
  -  if the resource already exists then the specification for that resource is
     replaced in it's entirety by the new resource specification.

  The output of the command indicates how many resources were successfully
  applied, and the error reason if an error occurred.

  The resources are applied in the order they are specified.  In the event of a
  failure applying a specific resource it is possible to work out which
  resource failed based on the number of resources successfully applied

  When applying a resource to perform an update, the complete resource spec
  must be provided, it is not sufficient to supply only the fields that are
  being updated.
`
	parsedArgs, err := docopt.Parse(doc, args, true, "", false, false)
	if err != nil {
		fmt.Printf("Invalid option: 'calicoctl %s'. Use flag '--help' to read about a specific subcommand.\n", strings.Join(args, " "))
		os.Exit(1)
	}
	if len(parsedArgs) == 0 {
		return
	}

	results := executeConfigCommand(parsedArgs, actionApply)
	log.Infof("results: %+v", results)

	if results.fileInvalid {
		fmt.Printf("Failed to execute command: %v\n", results.err)
		os.Exit(1)
	} else if results.numHandled == 0 {
		if results.numResources == 0 {
			fmt.Printf("No resources specified in file\n")
		} else if results.numResources == 1 {
			fmt.Printf("Failed to apply '%s' resource: %v\n", results.singleKind, results.err)
		} else if results.singleKind != "" {
			fmt.Printf("Failed to apply any '%s' resources: %v\n", results.singleKind, results.err)
		} else {
			fmt.Printf("Failed to apply any resources: %v\n", results.err)
		}
		os.Exit(1)
	} else if results.err == nil {
		if results.singleKind != "" {
			fmt.Printf("Successfully applied %d '%s' resource(s)\n", results.numHandled, results.singleKind)
		} else {
			fmt.Printf("Successfully applied %d resource(s)\n", results.numHandled)
		}
	} else {
		fmt.Printf("Partial success: ")
		if results.singleKind != "" {
			fmt.Printf("applied the first %d out of %d '%s' resources:\n",
				results.numHandled, results.numResources, results.singleKind)
		} else {
			fmt.Printf("applied the first %d out of %d resources:\n",
				results.numHandled, results.numResources)
		}
		fmt.Printf("Hit error: %v\n", results.err)
		os.Exit(1)
	}
}
