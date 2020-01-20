#!/bin/bash
cd $(dirname $0)
(
  cd lb
  ip netns exec lb2 go run *.go -c ../config/lb2-lb.yml
)
