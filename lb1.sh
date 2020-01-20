#!/bin/bash
cd $(dirname $0)
(
  cd lb
  ip netns exec lb1 go run *.go -c ../config/lb1-lb.yml
)
