output "cluster" {
  description = "Provisioned nodes, roles and network addresses."
  value = {
    name              = local.name
    region            = var.region
    availability_zone = var.availability_zone
    node_count        = length(aws_instance.node)
    nodes = {
      for key, instance in aws_instance.node : key => {
        role            = local.nodes[key].role
        instance_id     = instance.id
        instance_type   = instance.instance_type
        private_ip      = instance.private_ip
        public_ip       = instance.public_ip
        data_volume_ids = sort([for volume_key, volume in aws_ebs_volume.data : volume.id if local.data_volumes[volume_key].node == key])
      }
    }
  }
}

output "ssh_commands" {
  description = "One SSH command per node, for the operator that owns the private key."
  value = {
    for key, instance in aws_instance.node : key => "ssh ubuntu@${instance.public_ip}"
  }
}
