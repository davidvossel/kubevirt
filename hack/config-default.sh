binaries="cmd/virt-controller cmd/virt-launcher cmd/virt-handler cmd/virt-api cmd/virtctl cmd/virt-manifest cmd/fake-qemu-process cmd/virt-dhcp cmd/fake-dnsmasq-process cmd/test-api"
docker_images="cmd/virt-controller cmd/virt-launcher cmd/virt-handler cmd/virt-api cmd/virt-manifest images/haproxy images/iscsi-demo-target-tgtd images/vm-killer images/libvirt-kubevirt images/spice-proxy cmd/virt-migrator cmd/registry-disk-v1alpha images/cirros-registry-disk-demo cmd/virt-dhcp cmd/test-api"
optional_docker_images="cmd/registry-disk-v1alpha images/fedora-atomic-registry-disk-demo"
docker_prefix=kubevirt
docker_tag=${DOCKER_TAG:-latest}
manifest_templates="`ls ${KUBEVIRT_PATH}manifests/*.in`"
master_ip=192.168.200.2
master_port=8184
network_provider=weave
primary_nic=${primary_nic:-eth1}
primary_node_name=${primary_node_name:-master}
