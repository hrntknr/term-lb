#!/bin/bash
ip netns exec lb2 go run *.go -c config/lb2-lb.yml
