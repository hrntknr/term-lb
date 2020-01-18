#!/bin/bash
cd $(dirname $0)

ZEBRA_PATH=/usr/lib/frr/zebra

ip netns add server
ip netns add lb1
ip netns add lb2
ip netns add client

ip link add lb1-client type veth peer name client-lb1
ip link set lb1-client netns lb1
ip link set client-lb1 netns client

ip link add lb2-client type veth peer name client-lb2
ip link set lb2-client netns lb2
ip link set client-lb2 netns client

ip link add lb1-server type veth peer name server-lb1
ip link set lb1-server netns lb1
ip link set server-lb1 netns server

ip link add lb2-server type veth peer name server-lb2
ip link set lb2-server netns lb2
ip link set server-lb2 netns server


ip netns exec server ip link set dev lo up
ip netns exec server ip link set server-lb1 up
ip netns exec server ip link set server-lb2 up
ip netns exec server sysctl -w net.ipv6.conf.all.forwarding=1
ip netns exec server sysctl -w net.ipv4.conf.all.forwarding=1

ip netns exec lb1 ip link set dev lo up
ip netns exec lb1 ip link set lb1-server up
ip netns exec lb1 ip link set lb1-client up
ip netns exec lb1 sysctl -w net.ipv6.conf.all.forwarding=1
ip netns exec lb1 sysctl -w net.ipv4.conf.all.forwarding=1

ip netns exec lb2 ip link set dev lo up
ip netns exec lb2 ip link set lb2-server up
ip netns exec lb2 ip link set lb2-client up
ip netns exec lb2 sysctl -w net.ipv6.conf.all.forwarding=1
ip netns exec lb2 sysctl -w net.ipv4.conf.all.forwarding=1

ip netns exec client ip link set dev lo up
ip netns exec client ip link set client-lb1 up
ip netns exec client ip link set client-lb2 up
ip netns exec client sysctl -w net.ipv6.conf.all.forwarding=1
ip netns exec client sysctl -w net.ipv4.conf.all.forwarding=1

sleep 1

# ip netns exec server ip -6 addr del $(ip netns exec server ip addr show server-lb1 | grep inet6 | awk '{print $2}') dev server-lb1
# ip netns exec server ip -6 addr del $(ip netns exec server ip addr show server-lb2 | grep inet6 | awk '{print $2}') dev server-lb2
# ip netns exec lb1 ip -6 addr del $(ip netns exec lb1 ip addr show lb1-server | grep inet6 | awk '{print $2}') dev lb1-server
# ip netns exec lb1 ip -6 addr del $(ip netns exec lb1 ip addr show lb1-client | grep inet6 | awk '{print $2}') dev lb1-client
# ip netns exec lb2 ip -6 addr del $(ip netns exec lb2 ip addr show lb2-server | grep inet6 | awk '{print $2}') dev lb2-server
# ip netns exec lb2 ip -6 addr del $(ip netns exec lb2 ip addr show lb2-client | grep inet6 | awk '{print $2}') dev lb2-client
# ip netns exec client ip -6 addr del $(ip netns exec client ip addr show client-lb1 | grep inet6 | awk '{print $2}') dev client-lb1
# ip netns exec client ip -6 addr del $(ip netns exec client ip addr show client-lb2 | grep inet6 | awk '{print $2}') dev client-lb2

# ip netns exec server ip -6 addr add fe80::1/64 dev server-lb1
# ip netns exec server ip -6 addr add fe80::1/64 dev server-lb2
# ip netns exec lb1 ip -6 addr add fe80::2/64 dev lb1-server
# ip netns exec lb1 ip -6 addr add fe80::1/64 dev lb1-client
# ip netns exec lb2 ip -6 addr add fe80::2/64 dev lb2-server
# ip netns exec lb2 ip -6 addr add fe80::1/64 dev lb2-client
# ip netns exec client ip -6 addr add fe80::2/64 dev client-lb1
# ip netns exec client ip -6 addr add fe80::2/64 dev client-lb2

ip netns exec server ip -6 addr add fd12::1/60 dev server-lb1
ip netns exec server ip -6 addr add fd13::1/64 dev server-lb2
ip netns exec lb1 ip -6 addr add fd12::2/64 dev lb1-server
ip netns exec lb1 ip -6 addr add fd24::1/64 dev lb1-client
ip netns exec lb2 ip -6 addr add fd13::2/64 dev lb2-server
ip netns exec lb2 ip -6 addr add fd34::1/64 dev lb2-client
ip netns exec client ip -6 addr add fd24::2/64 dev client-lb1
ip netns exec client ip -6 addr add fd34::2/64 dev client-lb2

ip netns exec server ip addr add 192.168.0.1 dev lo
ip netns exec server ip -6 addr add fd00::1/128 dev lo

ip netns exec lb1 ip addr add 192.168.0.2 dev lo
ip netns exec lb1 ip -6 addr add fd00::2/128 dev lo
ip netns exec lb1 ip -6 addr add fd01::1/128 dev lo #vip

ip netns exec lb2 ip addr add 192.168.0.3 dev lo
ip netns exec lb2 ip -6 addr add fd00::3/128 dev lo
ip netns exec lb2 ip -6 addr add fd01::1/128 dev lo #vip

ip netns exec client ip addr add 192.168.0.4 dev lo
ip netns exec client ip -6 addr add fd00::4/128 dev lo

sleep 2

ip netns exec server ping -I server-lb1 ff02::1 -c 1 &
ip netns exec server ping -I server-lb2 ff02::1 -c 1 &

ip netns exec lb1 ping -I lb1-server ff02::1 -c 1 &
ip netns exec lb1 ping -I lb1-client ff02::1 -c 1 &

ip netns exec lb2 ping -I lb2-server ff02::1 -c 1 &
ip netns exec lb2 ping -I lb2-client ff02::1 -c 1 &

ip netns exec client ping -I client-lb1 ff02::1 -c 1 &
ip netns exec client ping -I client-lb2 ff02::1 -c 1 &

sleep 1

if [ ! -e ./run ]; then
  mkdir run
  chmod 777 run
fi

ip netns exec server $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/server_zebra.pid -z $(pwd)/run/server.api -d

ip netns exec lb1 $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/lb1_zebra.pid -z $(pwd)/run/lb1.api -d

ip netns exec lb2 $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/lb2_zebra.pid -z $(pwd)/run/lb2.api -d

ip netns exec client $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/client_zebra.pid -z $(pwd)/run/client.api -d

sleep 3

ip netns exec server ./gobgp/gobgpd -t yaml -f config/server.yml &
echo $! > run/server.pid

ip netns exec lb1 ./gobgp/gobgpd -t yaml -f config/lb1.yml &
echo $! > run/lb1.pid

ip netns exec lb2 ./gobgp/gobgpd -t yaml -f config/lb2.yml &
echo $! > run/lb2.pid

ip netns exec client ./gobgp/gobgpd -t yaml -f config/client.yml &
echo $! > run/client.pid

sleep 1

ip netns exec server ./gobgp/gobgp global rib add 192.168.0.1/32
ip netns exec server ./gobgp/gobgp global rib add fd00::1/128 -a ipv6
ip netns exec lb1 ./gobgp/gobgp global rib add 192.168.0.2/32
ip netns exec lb1 ./gobgp/gobgp global rib add fd00::2/128 -a ipv6
ip netns exec lb1 ./gobgp/gobgp global rib add fd01::1/128 -a ipv6
ip netns exec lb2 ./gobgp/gobgp global rib add 192.168.0.3/32
ip netns exec lb2 ./gobgp/gobgp global rib add fd00::3/128 -a ipv6
ip netns exec lb2 ./gobgp/gobgp global rib add fd01::1/128 -a ipv6
ip netns exec client ./gobgp/gobgp global rib add 192.168.0.4/32
ip netns exec client ./gobgp/gobgp global rib add fd00::4/128 -a ipv6
