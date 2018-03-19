#!/bin/sh

RT_TABLES_FILE=/etc/iproute2/rt_tables
K8S_TABLE_NR=242
K8S_TABLE_NAME=kubernetes

grep -q $K8S_TABLE_NAME $RT_TABLES_FILE
if [ $? -ne 0 ] ; then (echo $K8S_TABLE_NR $K8S_TABLE_NAME >> $RT_TABLES_FILE); fi

ip rule show | grep $K8S_TABLE_NAME
if [ $? -ne 0 ] ; then (ip rule add from all lookup $K8S_TABLE_NAME prio 1000);fi

source ./config.conf
