#!/bin/bash
ip netns exec lb1 ./gobgp/gobgp global rib del fc01::1/128 -a ipv6
ip netns exec lb2 ./gobgp/gobgp global rib add fc01::1/128 -a ipv6
