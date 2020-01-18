#!/bin/bash
ip netns exec lb1 go run *.go -c config/lb1-lb.yml
