#!/bin/sh

veth_usage(){
	echo "Usage: veth.sh <create|delete>"
}	

if [ "$#" = "0" ]; then
	veth_usage
elif [ "$1" = "create" ]; then
	ip link add dev veth1 type veth peer name veth2
	ip link set dev veth1 up
	ip link set dev veth2 up
	ip addr set dev veth1 10.11.0.1/24
	ip addr set dev veth2 10.11.0.2/24
elif [ "$1" = "delete" ]; then
	ip link del dev veth1
else
	veth_usage
fi

