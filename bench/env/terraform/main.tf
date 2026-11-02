locals {
  name = "${var.project}-${var.environment}"

  common_tags = merge(
    {
      Project     = var.project
      Environment = var.environment
      ManagedBy   = "terraform"
      Purpose     = "observaki-benchmark"
    },
    var.tags,
  )

  roles = {
    clickhouse = {
      count         = var.clickhouse_count
      instance_type = var.clickhouse_instance_type
      data_disks    = var.clickhouse_data_disks
      data_disk_gib = var.clickhouse_data_disk_gib
      throughput    = var.clickhouse_data_disk_throughput_mbps
      iops          = var.clickhouse_data_disk_iops
    }
    victoriametrics = {
      count         = var.victoriametrics_count
      instance_type = var.victoriametrics_instance_type
      data_disks    = var.victoriametrics_data_disks
      data_disk_gib = var.victoriametrics_data_disk_gib
      throughput    = var.victoriametrics_data_disk_throughput_mbps
      iops          = var.victoriametrics_data_disk_iops
    }
    gateway = {
      count         = var.gateway_count
      instance_type = var.gateway_instance_type
      data_disks    = var.gateway_data_disks
      data_disk_gib = var.gateway_data_disk_gib
      throughput    = var.gateway_data_disk_throughput_mbps
      iops          = var.gateway_data_disk_iops
    }
    nats = {
      count         = var.nats_count
      instance_type = var.nats_instance_type
      data_disks    = var.nats_data_disks
      data_disk_gib = var.nats_data_disk_gib
      throughput    = var.nats_data_disk_throughput_mbps
      iops          = var.nats_data_disk_iops
    }
    loadgen = {
      count         = var.loadgen_count
      instance_type = var.loadgen_instance_type
      data_disks    = var.loadgen_data_disks
      data_disk_gib = var.loadgen_data_disk_gib
      throughput    = var.loadgen_data_disk_throughput_mbps
      iops          = var.loadgen_data_disk_iops
    }
  }

  node_list = flatten([
    for role, config in local.roles : [
      for index in range(config.count) : {
        key           = "${role}-${format("%02d", index + 1)}"
        role          = role
        index         = index
        instance_type = config.instance_type
        data_disks    = config.data_disks
        data_disk_gib = config.data_disk_gib
        throughput    = config.throughput
        iops          = config.iops
      }
    ]
  ])
  nodes = { for node in local.node_list : node.key => node }

  data_volume_list = flatten([
    for node_key, node in local.nodes : [
      for index in range(node.data_disks) : {
        key        = "${node_key}-data-${format("%02d", index + 1)}"
        node       = node_key
        index      = index
        role       = node.role
        size       = node.data_disk_gib
        throughput = node.throughput
        iops       = node.iops
      }
    ]
  ])
  data_volumes = { for volume in local.data_volume_list : volume.key => volume }

  device_names = ["/dev/sdf", "/dev/sdg", "/dev/sdh", "/dev/sdi", "/dev/sdj", "/dev/sdk"]
}

resource "aws_vpc" "bench" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "${local.name}-vpc"
  }
}

resource "aws_subnet" "bench" {
  vpc_id                  = aws_vpc.bench.id
  cidr_block              = var.subnet_cidr
  availability_zone       = var.availability_zone
  map_public_ip_on_launch = true

  tags = {
    Name = "${local.name}-public"
  }
}

resource "aws_internet_gateway" "bench" {
  vpc_id = aws_vpc.bench.id

  tags = {
    Name = "${local.name}-igw"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.bench.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.bench.id
  }

  tags = {
    Name = "${local.name}-public"
  }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.bench.id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "bench" {
  name        = "${local.name}-sg"
  description = "Benchmark cluster: SSH from the operator and all traffic inside the cluster."
  vpc_id      = aws_vpc.bench.id

  ingress {
    description = "SSH from the operator"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.admin_cidr]
  }

  ingress {
    description = "All traffic inside the benchmark cluster"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
  }

  egress {
    description = "Package installation and operator access"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${local.name}-sg"
  }
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

resource "aws_key_pair" "bench" {
  key_name   = "${local.name}-key"
  public_key = trimspace(var.ssh_public_key)

  tags = {
    Name = "${local.name}-key"
  }
}

resource "aws_instance" "node" {
  for_each = local.nodes

  ami                         = data.aws_ami.ubuntu.id
  instance_type               = each.value.instance_type
  subnet_id                   = aws_subnet.bench.id
  vpc_security_group_ids      = [aws_security_group.bench.id]
  key_name                    = aws_key_pair.bench.key_name
  associate_public_ip_address = true
  user_data                   = templatefile("${path.module}/user_data.sh.tftpl", { hostname = "${local.name}-${each.key}" })
  user_data_replace_on_change = false

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_disk_gib
    encrypted             = true
    delete_on_termination = true
  }

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  tags = {
    Name      = "${local.name}-${each.key}"
    Role      = each.value.role
    NodeIndex = tostring(each.value.index + 1)
  }
}

resource "aws_ebs_volume" "data" {
  for_each = local.data_volumes

  availability_zone = var.availability_zone
  size              = each.value.size
  type              = "gp3"
  throughput        = each.value.throughput
  iops              = each.value.iops
  encrypted         = true

  tags = {
    Name = "${local.name}-${each.key}"
    Role = each.value.role
    Node = each.value.node
  }
}

resource "aws_volume_attachment" "data" {
  for_each = local.data_volumes

  device_name = local.device_names[each.value.index]
  volume_id   = aws_ebs_volume.data[each.key].id
  instance_id = aws_instance.node[each.value.node].id
}
