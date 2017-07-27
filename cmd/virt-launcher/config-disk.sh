#!/bin/bash

USER_DATA=$CONFIG_DISK_PATH/user-data
META_DATA=$CONFIG_DISK_PATH/meta-data
ISO=$CONFIG_DISK_PATH/$CONFIG_DISK_FILE

trap "rm -f $ISO" EXIT

groupadd --gid 1111 qemu 
if [ $? -ne 0 ]; then
	echo "failed to create qemu group"
	exit 1
fi

useradd --uid 1111 --gid 1111 qemu
if [ $? -ne 0 ]; then
	echo "failed to create qemu user"
	exit 1
fi

if ! [ -d "$CONFIG_DISK_PATH" ]; then
	echo "$CONFIG_DISK_PATH does not exist"
	exit 1
fi

echo "$USER_DATA_BASE64" | base64 -d > $USER_DATA
if [ $? -ne 0 ]; then
	echo "failed to decode cloud-init user-data"
	exit 1
fi

echo "$META_DATA_BASE64" | base64 -d > $META_DATA
if [ $? -ne 0 ]; then
	echo "failed to decode cloud-init meta-data"
	exit 1
fi

genisoimage -output $ISO -volid cidata -joliet -rock $USER_DATA $META_DATA
if [ $? -ne 0 ]; then
	echo "failed to generate configDisk ISO file"
	exit 1
fi

if ! [ -f $ISO ]; then
	echo "configDisk ISO file did not generate"
	exit 1
fi

rm -f $USER_DATA
rm -f $META_DATA

chown -R qemu:qemu $CONFIG_DISK_PATH
echo "Generated configDisk ISO at path $ISO"

touch /tmp/healthy

while true; do sleep 30; done
