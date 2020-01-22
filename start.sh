#!/bin/bash
cd $(dirname $0)

ZEBRA_PATH=/usr/lib/frr/zebra

ip netns add server
ip netns add lb1
ip netns add lb2
ip netns add router
ip netns add client

ip link add client-router type veth peer name router-client
ip link set client-router netns client
ip link set router-client netns router

ip link add lb1-router type veth peer name router-lb1
ip link set lb1-router netns lb1
ip link set router-lb1 netns router

ip link add lb2-router type veth peer name router-lb2
ip link set lb2-router netns lb2
ip link set router-lb2 netns router

ip link add lb1-lb2 type veth peer name lb2-lb1
ip link set lb1-lb2 netns lb1
ip link set lb2-lb1 netns lb2

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

ip netns exec lb1 ip link set dev lo up
ip netns exec lb1 ip link set lb1-server up
ip netns exec lb1 ip link set lb1-router up
ip netns exec lb1 ip link set lb1-lb2 up
ip netns exec lb1 sysctl -w net.ipv6.conf.all.forwarding=1

ip netns exec lb2 ip link set dev lo up
ip netns exec lb2 ip link set lb2-server up
ip netns exec lb2 ip link set lb2-router up
ip netns exec lb2 ip link set lb2-lb1 up
ip netns exec lb2 sysctl -w net.ipv6.conf.all.forwarding=1

ip netns exec router ip link set dev lo up
ip netns exec router ip link set router-lb1 up
ip netns exec router ip link set router-lb2 up
ip netns exec router ip link set router-client up
ip netns exec router sysctl -w net.ipv6.conf.all.forwarding=1

ip netns exec client ip link set dev lo up
ip netns exec client ip link set client-router up

sleep 1

# ip netns exec server ip -6 addr del $(ip netns exec server ip addr show server-lb1 | grep inet6 | awk '{print $2}') dev server-lb1
# ip netns exec server ip -6 addr del $(ip netns exec server ip addr show server-lb2 | grep inet6 | awk '{print $2}') dev server-lb2
# ip netns exec lb1 ip -6 addr del $(ip netns exec lb1 ip addr show lb1-server | grep inet6 | awk '{print $2}') dev lb1-server
# ip netns exec lb1 ip -6 addr del $(ip netns exec lb1 ip addr show lb1-router | grep inet6 | awk '{print $2}') dev lb1-router
# ip netns exec lb2 ip -6 addr del $(ip netns exec lb2 ip addr show lb2-server | grep inet6 | awk '{print $2}') dev lb2-server
# ip netns exec lb2 ip -6 addr del $(ip netns exec lb2 ip addr show lb2-router | grep inet6 | awk '{print $2}') dev lb2-router
# ip netns exec router ip -6 addr del $(ip netns exec router ip addr show router-lb1 | grep inet6 | awk '{print $2}') dev router-lb1
# ip netns exec router ip -6 addr del $(ip netns exec router ip addr show router-lb2 | grep inet6 | awk '{print $2}') dev router-lb2

# ip netns exec server ip -6 addr add fe80::1/64 dev server-lb1
# ip netns exec server ip -6 addr add fe80::1/64 dev server-lb2
# ip netns exec lb1 ip -6 addr add fe80::2/64 dev lb1-server
# ip netns exec lb1 ip -6 addr add fe80::1/64 dev lb1-router
# ip netns exec lb2 ip -6 addr add fe80::2/64 dev lb2-server
# ip netns exec lb2 ip -6 addr add fe80::1/64 dev lb2-router
# ip netns exec router ip -6 addr add fe80::2/64 dev router-lb1
# ip netns exec router ip -6 addr add fe80::2/64 dev router-lb2

ip netns exec server ip -6 addr add fc12::1/60 dev server-lb1
ip netns exec server ip -6 addr add fc13::1/64 dev server-lb2
ip netns exec lb1 ip -6 addr add fc12::2/64 dev lb1-server
ip netns exec lb1 ip -6 addr add fc24::1/64 dev lb1-router
ip netns exec lb1 ip -6 addr add fc23::1/64 dev lb1-lb2
ip netns exec lb2 ip -6 addr add fc13::2/64 dev lb2-server
ip netns exec lb2 ip -6 addr add fc34::1/64 dev lb2-router
ip netns exec lb2 ip -6 addr add fc23::2/64 dev lb2-lb1
ip netns exec router ip -6 addr add fc24::2/64 dev router-lb1
ip netns exec router ip -6 addr add fc34::2/64 dev router-lb2
ip netns exec router ip -6 addr add fc45::1/64 dev router-client
ip netns exec client ip -6 addr add fc45::2/64 dev client-router

ip netns exec server ip -6 addr add fc00::1/128 dev lo

ip netns exec lb1 ip -6 addr add fc00::2/128 dev lo
ip netns exec lb1 ip -6 addr add fc01::1/128 dev lo #vip
ip netns exec lb1 ip -6 route add local fc0a::/64 dev lo #anyIP

ip netns exec lb2 ip -6 addr add fc00::3/128 dev lo
ip netns exec lb2 ip -6 addr add fc01::1/128 dev lo #vip
ip netns exec lb2 ip -6 route add local fc0b::/64 dev lo #anyIP

ip netns exec router ip -6 addr add fc00::4/128 dev lo

sleep 2

ip netns exec server ping -I server-lb1 ff02::1 -c 1 &
ip netns exec server ping -I server-lb2 ff02::1 -c 1 &

ip netns exec lb1 ping -I lb1-server ff02::1 -c 1 &
ip netns exec lb1 ping -I lb1-router ff02::1 -c 1 &

ip netns exec lb2 ping -I lb2-server ff02::1 -c 1 &
ip netns exec lb2 ping -I lb2-router ff02::1 -c 1 &

ip netns exec router ping -I router-lb1 ff02::1 -c 1 &
ip netns exec router ping -I router-lb2 ff02::1 -c 1 &
ip netns exec router ping -I router-client ff02::1 -c 1 &

ip netns exec client ping -I client-router ff02::1 -c 1 &

sleep 1

ip netns exec client ip -6 route add ::/0 via fc45::1

if [ ! -e ./run ]; then
  mkdir run
  chmod 777 run
fi

ip netns exec server $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/server_zebra.pid -z $(pwd)/run/server.api -d

ip netns exec lb1 $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/lb1_zebra.pid -z $(pwd)/run/lb1.api -d

ip netns exec lb2 $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/lb2_zebra.pid -z $(pwd)/run/lb2.api -d

ip netns exec router $ZEBRA_PATH -A 127.0.0.1 -i $(pwd)/run/router_zebra.pid -z $(pwd)/run/router.api -d

sleep 3

# ip netns exec server python3 ./server.py &
echo $! > run/server_http.pid

ip netns exec server ./gobgp/gobgpd -t yaml -f config/server.yml &
echo $! > run/server.pid

ip netns exec lb1 ./gobgp/gobgpd -t yaml -f config/lb1.yml &
echo $! > run/lb1.pid

ip netns exec lb2 ./gobgp/gobgpd -t yaml -f config/lb2.yml &
echo $! > run/lb2.pid

ip netns exec router ./gobgp/gobgpd -t yaml -f config/router.yml &
echo $! > run/router.pid

sleep 1

ip netns exec server ./gobgp/gobgp global rib add fc12::/64 -a ipv6
ip netns exec server ./gobgp/gobgp global rib add fc13::/64 -a ipv6

ip netns exec lb1 ./gobgp/gobgp global rib add fc12::/64 -a ipv6
ip netns exec lb1 ./gobgp/gobgp global rib add fc24::/64 -a ipv6

ip netns exec lb2 ./gobgp/gobgp global rib add fc13::/64 -a ipv6
ip netns exec lb2 ./gobgp/gobgp global rib add fc34::/64 -a ipv6

ip netns exec router ./gobgp/gobgp global rib add fc24::/64 -a ipv6
ip netns exec router ./gobgp/gobgp global rib add fc34::/64 -a ipv6
ip netns exec router ./gobgp/gobgp global rib add fc45::/64 -a ipv6

ip netns exec server ./gobgp/gobgp global rib add 192.168.0.1/32
ip netns exec server ./gobgp/gobgp global rib add fc00::1/128 -a ipv6
ip netns exec lb1 ./gobgp/gobgp global rib add 192.168.0.2/32
ip netns exec lb1 ./gobgp/gobgp global rib add fc00::2/128 -a ipv6
ip netns exec lb1 ./gobgp/gobgp global rib add fc01::1/128 -a ipv6 #vip
ip netns exec lb2 ./gobgp/gobgp global rib add 192.168.0.3/32
ip netns exec lb2 ./gobgp/gobgp global rib add fc00::3/128 -a ipv6
# ip netns exec lb2 ./gobgp/gobgp global rib add fc01::1/128 -a ipv6 #vip
ip netns exec router ./gobgp/gobgp global rib add 192.168.0.4/32
ip netns exec router ./gobgp/gobgp global rib add fc00::4/128 -a ipv6

ip netns exec lb1 ip6tables -A OUTPUT -p tcp --tcp-flags ALL RST --sport 8080 -j DROP
ip netns exec lb2 ip6tables -A OUTPUT -p tcp --tcp-flags ALL RST --sport 8080 -j DROP
