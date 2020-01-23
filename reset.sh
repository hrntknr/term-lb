#!/bin/bash
ip netns exec lb1 ./gobgp/gobgp global rib del all -a 6
ip netns exec lb2 ./gobgp/gobgp global rib del all -a 6

ip netns exec lb1 ./gobgp/gobgp global rib add fc12::/64 -a 6
ip netns exec lb1 ./gobgp/gobgp global rib add fc24::/64 -a 6

ip netns exec lb1 ./gobgp/gobgp global rib add fc00::2/128 -a 6
ip netns exec lb1 ./gobgp/gobgp global rib add fc01::1/128 -a 6 #vip
ip netns exec lb1 ./gobgp/gobgp global rib add fca1::/64 -a 6 #anyIP

ip netns exec lb2 ./gobgp/gobgp global rib add fc13::/64 -a 6
ip netns exec lb2 ./gobgp/gobgp global rib add fc34::/64 -a 6

ip netns exec lb2 ./gobgp/gobgp global rib add fc11::3/128 -a 6
# ip netns exec lb2 ./gobgp/gobgp global rib add fc01::1/128 -a 6 #vip
ip netns exec lb2 ./gobgp/gobgp global rib add fca2::/64 -a 6 #anyIP
