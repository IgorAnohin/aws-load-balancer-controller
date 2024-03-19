#!/bin/bash

VERSION=31

make docker-build

image_id=$(docker images | grep "2.7.0" | awk '{print $3}')

docker tag $image_id igranokhin/custom-aws-load-balancer-controller:debug.$VERSION
docker push igranokhin/custom-aws-load-balancer-controller:debug.$VERSION
