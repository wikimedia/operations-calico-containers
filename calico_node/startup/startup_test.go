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

package main

import (
	"log"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/projectcalico/libcalico-go/lib/api"
	"github.com/projectcalico/libcalico-go/lib/client"
	"github.com/projectcalico/libcalico-go/lib/ipip"
	"github.com/projectcalico/libcalico-go/lib/testutils"
)

var exitCode int

func fakeExitFunction(ec int) {
	exitCode = ec
}

var _ = Describe("Non-etcd related tests", func() {

	Describe("Logging tests", func() {
		Context("Test message", func() {
			message("Test message %d, %s", 4, "END")
		})
		Context("Test warning", func() {
			warning("Test message %d, %s", 4, "END")
		})
		Context("Test fatal", func() {
			fatal("Test message %d, %s", 4, "END")
		})
	})

	Describe("Termination tests", func() {
		exitCode = 0
		Context("Test termination", func() {
			oldExit := exitFunction
			exitFunction = fakeExitFunction
			defer func() { exitFunction = oldExit }()
			terminate()
			It("should have terminated", func() {
				Expect(exitCode).To(Equal(1))
			})
		})
	})
})

type EnvItem struct {
	key   string
	value string
}

var _ = Describe("FV tests against a real etcd", func() {
	changedEnvVars := []string{"CALICO_IPV4POOL_CIDR", "CALICO_IPV6POOL_CIDR", "NO_DEFAULT_POOLS", "CALICO_IPV4POOL_IPIP"}

	BeforeEach(func() {
		for _, envName := range changedEnvVars {
			os.Unsetenv(envName)
		}
	})
	AfterEach(func() {
		for _, envName := range changedEnvVars {
			os.Unsetenv(envName)
		}
	})

	DescribeTable("Test IP pool env variables",
		func(envList []EnvItem, expectedIPv4 string, expectedIPv6 string, expectIpv4IpipMode string) {
			// Create a new client.
			cfg, _ := client.LoadClientConfigFromEnvironment()
			c := testutils.CreateCleanClient(*cfg)

			// Set the env variables specified.
			for _, env := range envList {
				os.Setenv(env.key, env.value)
			}
			poolList, err := c.IPPools().List(api.IPPoolMetadata{})
			Expect(poolList.Items).To(BeEmpty())

			// Run the UUT.
			configureIPPools(c)

			// Get the IPPool list.
			poolList, err = c.IPPools().List(api.IPPoolMetadata{})
			Expect(err).NotTo(HaveOccurred())
			log.Println("Get pool list returns: ", poolList.Items)

			// Look through the pool for the expected data.
			foundv4Expected := false
			foundv6Expected := false
			for _, pool := range poolList.Items {
				if pool.Metadata.CIDR.String() == expectedIPv4 {
					foundv4Expected = true
				}
				if pool.Metadata.CIDR.String() == expectedIPv6 {
					foundv6Expected = true
				}
				if pool.Metadata.CIDR.Version() == 6 {
					// Expect IPIP on IPv6 to be disabled
					if pool.Spec.IPIP != nil {
						Expect(pool.Spec.IPIP.Enabled).To(BeFalse())
					}
				} else {
					// off is not a real mode value but use it instead of empty string
					if expectIpv4IpipMode == "off" {
						if pool.Spec.IPIP != nil {
							Expect(pool.Spec.IPIP.Enabled).To(BeFalse())
						}
					} else {
						Expect(pool.Spec.IPIP.Enabled).To(BeTrue())
						Expect(pool.Spec.IPIP.Mode).To(Equal(ipip.Mode(expectIpv4IpipMode)))
					}
				}
			}
			Expect(foundv4Expected).To(BeTrue(),
				"Expected %s to be in Pools", expectedIPv4)
			Expect(foundv6Expected).To(BeTrue(),
				"Expected %s to be in Pools", expectedIPv6)
		},

		Entry("No env variables set", []EnvItem{},
			"192.168.0.0/16", "fd80:24e2:f998:72d6::/64", "off"),
		Entry("IPv4 Pool env var set",
			[]EnvItem{{"CALICO_IPV4POOL_CIDR", "172.16.0.0/24"}},
			"172.16.0.0/24", "fd80:24e2:f998:72d6::/64", "off"),
		Entry("IPv6 Pool env var set",
			[]EnvItem{{"CALICO_IPV6POOL_CIDR", "fdff:ffff:ffff:ffff:ffff::/80"}},
			"192.168.0.0/16", "fdff:ffff:ffff:ffff:ffff::/80", "off"),
		Entry("Both IPv4 and IPv6 Pool env var set",
			[]EnvItem{
				{"CALICO_IPV4POOL_CIDR", "172.16.0.0/24"},
				{"CALICO_IPV6POOL_CIDR", "fdff:ffff:ffff:ffff:ffff::/80"},
			},
			"172.16.0.0/24", "fdff:ffff:ffff:ffff:ffff::/80", "off"),
		Entry("CALICO_IPV4POOL_IPIP set off", []EnvItem{{"CALICO_IPV4POOL_IPIP", "off"}},
			"192.168.0.0/16", "fd80:24e2:f998:72d6::/64", "off"),
		Entry("CALICO_IPV4POOL_IPIP set always", []EnvItem{{"CALICO_IPV4POOL_IPIP", "always"}},
			"192.168.0.0/16", "fd80:24e2:f998:72d6::/64", "always"),
		Entry("CALICO_IPV4POOL_IPIP set cross-subnet", []EnvItem{{"CALICO_IPV4POOL_IPIP", "cross-subnet"}},
			"192.168.0.0/16", "fd80:24e2:f998:72d6::/64", "cross-subnet"),
		Entry("IPv6 Pool and IPIP set",
			[]EnvItem{
				{"CALICO_IPV6POOL_CIDR", "fdff:ffff:ffff:ffff:ffff::/80"},
				{"CALICO_IPV4POOL_IPIP", "always"},
			},
			"192.168.0.0/16", "fdff:ffff:ffff:ffff:ffff::/80", "always"),
	)

	Describe("Test NO_DEFAULT_POOLS env variable", func() {
		Context("Should have no pools defined", func() {
			// Create a new client.
			cfg, _ := client.LoadClientConfigFromEnvironment()
			c := testutils.CreateCleanClient(*cfg)

			// Set the env variables specified.
			os.Setenv("NO_DEFAULT_POOLS", "true")

			// Run the UUT.
			configureIPPools(c)

			// Get the IPPool list.
			poolList, err := c.IPPools().List(api.IPPoolMetadata{})
			It("should be able to access the IP pool list", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			log.Println("Get pool list returns: ", poolList.Items)

			It("should have no IP pools", func() {
				Expect(poolList.Items).To(BeEmpty(), "Environment %#v", os.Environ())
			})
		})
	})

	DescribeTable("Test IP pool env variables that cause exit",
		func(envList []EnvItem) {
			my_ec := 0
			oldExit := exitFunction
			exitFunction = func(ec int) { my_ec = ec }
			defer func() { exitFunction = oldExit }()

			// Create a new client.
			cfg, _ := client.LoadClientConfigFromEnvironment()
			c := testutils.CreateCleanClient(*cfg)

			// Set the env variables specified.
			for _, env := range envList {
				os.Setenv(env.key, env.value)
			}

			// Run the UUT.
			configureIPPools(c)

			Expect(my_ec).To(Equal(1))
		},

		Entry("Bad IPv4 Pool CIDR", []EnvItem{{"CALICO_IPV4POOL_CIDR", "172.16.0.0a/24"}}),
		Entry("Too small IPv4 Pool CIDR", []EnvItem{{"CALICO_IPV4POOL_CIDR", "172.16.0.0/27"}}),
		Entry("Single IPv4 is too small for a pool CIDR", []EnvItem{{"CALICO_IPV4POOL_CIDR", "10.0.0.0/32"}}),
		Entry("Small IPv6 is too small for a pool CIDR", []EnvItem{{"CALICO_IPV6POOL_CIDR", "fd00::/123"}}),
		Entry("Bad IPv4 Pool with good IPv6 Pool env var set",
			[]EnvItem{
				{"CALICO_IPV4POOL_CIDR", "172.16.0.0a/24"},
				{"CALICO_IPV6POOL_CIDR", "fdff:ffff:ffff:ffff:ffff::/80"},
			}),
		Entry("Invalid Env Var combo",
			[]EnvItem{
				{"NO_DEFAULT_POOLS", "true"},
				{"CALICO_IPV4POOL_CIDR", "172.16.0.0/24"},
			}),
		Entry("Bad IPv4 Pool IPIP Mode", []EnvItem{{"CALICO_IPV4POOL_IPIP", "badVal"}}),
		Entry("v6 Address in IPv4 Pool CIDR",
			[]EnvItem{{"CALICO_IPV4POOL_CIDR", "fdff:ffff:ffff:ffff:ffff::/80"}}),
		Entry("v4 Address in IPv6 Pool CIDR",
			[]EnvItem{{"CALICO_IPV6POOL_CIDR", "172.16.0.0/24"}}),
	)

	Describe("Test we properly wait for the etcd datastore", func() {
		// Create a new client.
		cfg, _ := client.LoadClientConfigFromEnvironment()
		c := testutils.CreateCleanClient(*cfg)

		// Wait for a connection.
		done := make(chan bool)
		go func() {
			// Wait for a connection.
			waitForConnection(c)

			// Once connected, indicate that we connected on the channel.
			done <- true
		}()

		// Wait for a done signal to indicate that we've connected to the datastore.
		// If we don't receive one in 5 seconds, then fail.
		count := 0
		for {
			select {
			case <-done:
				// Finished.  Success!
				log.Println("Connected to datastore")
				return
			default:
				count++
				time.Sleep(1 * time.Second)
				if count > 5 {
					log.Fatal("Timed out waiting for datastore after 5 seconds")
				}
			}
		}
	})
})
