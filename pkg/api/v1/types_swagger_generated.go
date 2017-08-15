// Automatically generated by swagger-doc. DO NOT EDIT!

package v1

func (VM) SwaggerDoc() map[string]string {
	return map[string]string{
		"":       "VM is *the* VM Definition. It represents a virtual machine in the runtime environment of kubernetes.",
		"spec":   "VM Spec contains the VM specification.",
		"status": "Status is the high level overview of how the VM is doing. It contains information available to controllers and users.",
	}
}

func (VMList) SwaggerDoc() map[string]string {
	return map[string]string{
		"": "VMList is a list of VMs",
	}
}

func (VMSpec) SwaggerDoc() map[string]string {
	return map[string]string{
		"":             "VMSpec is a description of a VM. Not to be confused with api.DomainSpec in virt-handler.\nIt is expected that v1.DomainSpec will be merged into this structure.",
		"domain":       "Domain is the actual libvirt domain.",
		"cloudInit":    "The cloud-init data associated with this VM.",
		"nodeSelector": "If labels are specified, only nodes marked with all of these labels are considered when scheduling the VM.",
	}
}

func (VMStatus) SwaggerDoc() map[string]string {
	return map[string]string{
		"":                  "VMStatus represents information about the status of a VM. Status may trail the actual\nstate of a system.",
		"nodeName":          "NodeName is the name where the VM is currently running.",
		"migrationNodeName": "MigrationNodeName is the node where the VM is live migrating to.",
		"conditions":        "Conditions are specific points in VM's pod runtime.",
		"phase":             "Phase is the status of the VM in kubernetes world. It is not the VM status, but partially correlates to it.",
		"graphics":          "Graphics represent the details of available graphical consoles.",
	}
}

func (VMGraphics) SwaggerDoc() map[string]string {
	return map[string]string{}
}

func (VMCondition) SwaggerDoc() map[string]string {
	return map[string]string{}
}

func (Spice) SwaggerDoc() map[string]string {
	return map[string]string{}
}

func (SpiceInfo) SwaggerDoc() map[string]string {
	return map[string]string{}
}

func (Migration) SwaggerDoc() map[string]string {
	return map[string]string{
		"": "A Migration is a job that moves a Virtual Machine from one node to another",
	}
}

func (MigrationSpec) SwaggerDoc() map[string]string {
	return map[string]string{
		"":             "MigrationSpec is a description of a VM Migration\nFor example \"destinationNodeName\": \"testvm\" will migrate a VM called \"testvm\" in the namespace \"default\"",
		"selector":     "Criterias for selecting the VM to migrate.\nFor example\nselector:\n  name: testvm\nwill select the VM `testvm` for migration",
		"nodeSelector": "Criteria to use when selecting the destination for the migration\nfor example, to select by the hostname, specify `kubernetes.io/hostname: master`\nother possible choices include the hardware required to run the vm or\nor lableing of the nodes to indicate their roles in larger applications.\nexamples:\ndisktype: ssd,\nrandomGenerator: /dev/random,\nrandomGenerator: superfastdevice,\napp: mysql,\nlicensedForServiceX: true\nNote that these selectors are additions to the node selectors on the VM itself and they must not exist on the VM.\nIf they are conflicting with the VM, no migration will be started.",
	}
}

func (VMSelector) SwaggerDoc() map[string]string {
	return map[string]string{
		"name": "Name of the VM to migrate",
	}
}

func (MigrationStatus) SwaggerDoc() map[string]string {
	return map[string]string{
		"": "MigrationStatus is the last reported status of a VM Migratrion. Status may trail the actual\nstate of a migration.",
	}
}

func (MigrationList) SwaggerDoc() map[string]string {
	return map[string]string{
		"": "A list of Migrations",
	}
}
