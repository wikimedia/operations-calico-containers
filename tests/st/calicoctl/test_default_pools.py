# Copyright (c) 2015-2016 Tigera, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
import functools
import logging

import netaddr
import yaml
from nose_parameterized import parameterized

from tests.st.test_base import TestBase
from tests.st.utils.docker_host import DockerHost, CLUSTER_STORE_DOCKER_OPTIONS, NODE_CONTAINER_NAME
from tests.st.utils.exceptions import CommandExecError
from tests.st.utils.utils import get_ip, wipe_etcd, retry_until_success

_log = logging.getLogger(__name__)
_log.setLevel(logging.DEBUG)


class TestDefaultPools(TestBase):
    @classmethod
    def setUpClass(cls):
        # First, create a (fake) host to run things in
        cls.host = DockerHost("host",
                              additional_docker_options=CLUSTER_STORE_DOCKER_OPTIONS,
                              start_calico=False,
                              dind=False)

    def setUp(self):
        try:
            self.host.execute("docker rm -f calico-node")
        except CommandExecError:
            # Presumably calico-node wasn't running
            pass
        wipe_etcd(get_ip())

    @classmethod
    def tearDownClass(cls):
            cls.host.cleanup()

    @parameterized.expand([
        (False, "CALICO_IPV4POOL_CIDR", "10.0.0.0/27", 0, None, "Too small"),
        (False, "CALICO_IPV4POOL_CIDR", "10.0.0.0/32", 0, None, "Too small, but legal CIDR"),
        (False, "CALICO_IPV4POOL_CIDR", "10.0.0.0/33", 0, None, "Impossible CIDR"),
        (False, "CALICO_IPV4POOL_CIDR", "256.0.0.0/24", 0, None, "Invalid IP"),
        (True, "CALICO_IPV4POOL_CIDR", "10.0.0.0/24", 2, None, "Typical non-default pool"),
        (True, "CALICO_IPV4POOL_CIDR", "10.0.0.0/26", 2, None, "Smallest legal pool"),
        (True, "CALICO_IPV6POOL_CIDR", "fd00::/122", 2, None, "Smallest legal pool"),
        (False, "CALICO_IPV6POOL_CIDR", "fd00::/123", 0, None, "Too small"),
        (False, "CALICO_IPV6POOL_CIDR", "fd00::/128", 0, None, "Too small, but legal CIDR"),
        (False, "CALICO_IPV6POOL_CIDR", "fd00::/129", 0, None, "Impossible CIDR"),
        (True, "CALICO_IPV4POOL_CIDR", "10.0.0.0/24", 2, "cross-subnet", "Typ. non-def pool, IPIP"),
        (True, "CALICO_IPV4POOL_CIDR", "10.0.0.0/24", 2, "always", "Typ. non-default pool, IPIP"),
        (True, "CALICO_IPV4POOL_CIDR", "10.0.0.0/24", 2, "off", "Typical pool, explicitly no IPIP"),
        (True, "CALICO_IPV6POOL_CIDR", "fd00::/122", 2, "always", "IPv6 - IPIP not permitted"),
        (True, "CALICO_IPV6POOL_CIDR", "fd00::/122", 2, "cross-subnet", "IPv6 - IPIP not allowed"),
        (True, "CALICO_IPV6POOL_CIDR", "fd00::/122", 2, "off", "IPv6, IPIP explicitly off"),
        (False, "CALICO_IPV6POOL_CIDR", "fd00::/122", 0, "junk", "Invalid IPIP value"),
        (False, "CALICO_IPV4POOL_CIDR", "10.0.0.0/24", 0, "reboot", "Invalid IPIP value"),
        (False, "CALICO_IPV4POOL_CIDR", "0.0.0.0/0", 0, None, "Invalid, link local address"),
        (False, "CALICO_IPV6POOL_CIDR", "::/0", 0, None, "Invalid, link local address"),
        (True, "CALICO_IPV6POOL_CIDR", "fd80::0:0/120", 2, None, "Valid, but non-canonical form"),
        (False, "CALICO_IPV6POOL_CIDR", "1.2.3.4/24", 0, None, "Wrong type"),
        (False, "CALICO_IPV4POOL_CIDR", "fd00::/24", 0, None, "Wrong type"),
        (True, "CALICO_IPV6POOL_CIDR", "::0:a:b:c:d:e:0/120", 2, None, "Valid, non-canonical form"),
        (False, "CALICO_IPV4POOL_CIDR", "1.2/16", 0, None, "Valid, unusual form"),
    ])
    def test_default_pools(self, success_expected, param, value, exp_num_pools, ipip, description):
        """
        Test that the various options for default pools work correctly
        """
        _log.debug("Test description: %s", description)
        # Get command line for starting docker
        output = self.host.calicoctl("node run --dryrun --node-image=%s" % NODE_CONTAINER_NAME)
        base_command = output.split('\n')[-4].rstrip()

        # Modify command line to add the options we want to test
        env_inserts = "-e %s=%s " % (param, value)
        if ipip is not None:
            env_inserts += "-e CALICO_IPV4POOL_IPIP=%s " % ipip
        prefix, _, suffix = base_command.partition("-e")
        command = prefix + env_inserts + "-e" + suffix

        # Start calico-docker
        self.host.execute(command)

        if not success_expected:
            # check for "Calico node failed to start"
            self.wait_for_node_log("Calico node failed to start")
            return

        # Check we started OK
        self.wait_for_node_log("Calico node started successfully")
        # check the expected pool is present
        pools_output = self.host.calicoctl("get ippool -o yaml")
        pools_dict = yaml.safe_load(pools_output)
        cidrs = [pool['metadata']['cidr'] for pool in pools_dict]
        # Convert to canonical form
        value = str(netaddr.IPNetwork(value))
        assert value in cidrs, "Didn't find %s in %s" % (value, cidrs)

        # Dump pools and attempt to load them with calicoctl (to confirm consistency)
        self.host.calicoctl("get ippool -o yaml > testfile.yaml")
        self.host.calicoctl("apply -f testfile.yaml")

        assert len(pools_dict) == exp_num_pools, \
            "Expected %s pools, found %s. %s" % (exp_num_pools, len(pools_dict), pools_dict)

        # Grab the pool of interest
        pool = pools_dict[cidrs.index(value)]
        other_pool = None
        # And grab the other pool if any
        if len(pools_dict) > 1:
            pools_dict.remove(pool)
            other_pool = pools_dict[0]
        # Check IPIP setting if we're doing IPv4
        if ipip in ["cross-subnet", "always"] and param == "CALICO_IPV4POOL_CIDR":
            assert pool['spec']['ipip']['enabled'] is True, \
                "Didn't find ipip enabled in pool %s" % pool
            assert pool['spec']['ipip']['mode'] == ipip, \
                "Didn't find ipip mode in pool %s" % pool
        if ipip in [None, "off"] or param == "CALICO_IPV6POOL_CIDR":
            assert 'ipip' not in pool['spec']
        if ipip in ["cross-subnet", "always"] and param == "CALICO_IPV6POOL_CIDR":
            assert other_pool['spec']['ipip']['enabled'] is True, \
                "Didn't find ipip enabled in pool %s" % pool
            assert other_pool['spec']['ipip']['mode'] == ipip, \
                "Didn't find ipip mode in pool %s" % pool

        # Check NAT setting
        assert pool['spec']['nat-outgoing'] is True, "Didn't find nat enabled in pool %s" % pool

    def test_no_default_pools(self):
        """
        Test that NO_DEFAULT_POOLS works correctly
        """
        # Start calico-docker
        self.host.start_calico_node(options="--no-default-ippools")
        self.wait_for_node_log("Calico node started successfully")
        # check the expected pool is present
        pools_output = self.host.calicoctl("get ippool -o yaml")
        pools_dict = yaml.safe_load(pools_output)
        assert pools_dict == [], "Pools not empty: %s" % pools_dict

    def assert_calico_node_log_contains(self, expected_string):
        assert expected_string in self.host.execute("docker logs calico-node"), \
            "Didn't find %s in start log" % expected_string

    def wait_for_node_log(self, expected_log):
        check = functools.partial(self.assert_calico_node_log_contains, expected_log)
        retry_until_success(check, 5, ex_class=AssertionError)
