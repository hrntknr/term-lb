cat run/server.pid | xargs kill -term
cat run/lb1.pid | xargs kill -term
cat run/lb2.pid | xargs kill -term
cat run/router.pid | xargs kill -term

cat run/server_zebra.pid | xargs kill -term
cat run/lb1_zebra.pid | xargs kill -term
cat run/lb2_zebra.pid | xargs kill -term
cat run/router_zebra.pid | xargs kill -term

ip netns delete server
ip netns delete lb1
ip netns delete lb2
ip netns delete router
ip netns delete client
