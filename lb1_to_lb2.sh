#!/bin/bash
ip netns exec lb1 ./gobgp/gobgp global rib del fd01::1/128 -a ipv6
ip netns exec lb2 ./gobgp/gobgp global rib add fd01::1/128 -a ipv6
