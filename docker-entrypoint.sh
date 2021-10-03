#!/bin/sh

# solution to access host machine from within a docker container.
# source: https://github.com/bufferings/docker-access-host

HOST_DOMAIN="host.docker.internal"
ping -q -c1 $HOST_DOMAIN > /dev/null 2>&1
if [ $? -ne 0 ]; then
  HOST_IP=$(ip route | awk 'NR==1 {print $3}')
  echo -e "$HOST_IP\t$HOST_DOMAIN" >> /etc/hosts
fi
cat /etc/hosts
./transmission-bot