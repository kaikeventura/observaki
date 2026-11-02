variable "project" {
  type        = string
  default     = "observaki-bench"
  description = "Prefix for every resource name and tag."
}

variable "environment" {
  type        = string
  default     = "t1"
  description = "Environment suffix; the T1 reference uses t1."
}

variable "region" {
  type        = string
  default     = "us-east-1"
  description = "AWS region. The profile is the source of truth; cluster.sh generates this value."
}

variable "availability_zone" {
  type        = string
  default     = "us-east-1a"
  description = "Single availability zone used by the benchmark. Cross-AZ traffic is deliberately excluded."
}

variable "vpc_cidr" {
  type        = string
  default     = "10.42.0.0/16"
  description = "CIDR of the dedicated benchmark VPC."
}

variable "subnet_cidr" {
  type        = string
  default     = "10.42.1.0/24"
  description = "CIDR of the public benchmark subnet."
}

variable "admin_cidr" {
  type        = string
  description = "CIDR allowed to SSH into the nodes. Never use 0.0.0.0/0."

  validation {
    condition     = can(cidrhost(var.admin_cidr, 0))
    error_message = "admin_cidr must be a valid CIDR such as 203.0.113.10/32."
  }
}

variable "ssh_public_key" {
  type        = string
  description = "Public key installed on every node. The private key stays with the operator."

  validation {
    condition     = length(trimspace(var.ssh_public_key)) > 0
    error_message = "ssh_public_key is required."
  }
}

variable "root_disk_gib" {
  type        = number
  default     = 100
  description = "gp3 root volume size for every node."
}

variable "tags" {
  type        = map(string)
  default     = {}
  description = "Extra tags merged into every resource."
}

variable "clickhouse_count" {
  type        = number
  default     = 6
  description = "ClickHouse nodes (reference contract: 6-12)."

  validation {
    condition     = var.clickhouse_count >= 6 && var.clickhouse_count <= 12
    error_message = "clickhouse_count must be between 6 and 12."
  }
}

variable "clickhouse_instance_type" {
  type        = string
  default     = "m6i.4xlarge"
  description = "ClickHouse instance type (16 vCPU, 64 GiB reference)."
}

variable "clickhouse_data_disks" {
  type        = number
  default     = 2
  description = "Data volumes per ClickHouse node."

  validation {
    condition     = var.clickhouse_data_disks >= 1
    error_message = "clickhouse_data_disks must be at least 1."
  }
}

variable "clickhouse_data_disk_gib" {
  type        = number
  default     = 16384
  description = "Size of each ClickHouse data volume. AWS gp3 caps a volume at 16384 GiB."

  validation {
    condition     = var.clickhouse_data_disk_gib >= 1 && var.clickhouse_data_disk_gib <= 16384
    error_message = "clickhouse_data_disk_gib must be between 1 and 16384."
  }
}

variable "clickhouse_data_disk_throughput_mbps" {
  type        = number
  default     = 500
  description = "gp3 throughput per ClickHouse data volume."

  validation {
    condition     = var.clickhouse_data_disk_throughput_mbps >= 125 && var.clickhouse_data_disk_throughput_mbps <= 1000
    error_message = "gp3 throughput must be between 125 and 1000 MB/s."
  }
}

variable "clickhouse_data_disk_iops" {
  type        = number
  default     = 16000
  description = "gp3 IOPS per ClickHouse data volume."

  validation {
    condition     = var.clickhouse_data_disk_iops >= 3000 && var.clickhouse_data_disk_iops <= 16000
    error_message = "gp3 IOPS must be between 3000 and 16000."
  }
}

variable "victoriametrics_count" {
  type        = number
  default     = 3
  description = "VictoriaMetrics nodes (reference contract: exactly 3)."

  validation {
    condition     = var.victoriametrics_count == 3
    error_message = "victoriametrics_count must be exactly 3."
  }
}

variable "victoriametrics_instance_type" {
  type        = string
  default     = "m6i.4xlarge"
  description = "VictoriaMetrics instance type (16 vCPU, 64 GiB reference)."
}

variable "victoriametrics_data_disks" {
  type        = number
  default     = 1
  description = "Data volumes per VictoriaMetrics node."

  validation {
    condition     = var.victoriametrics_data_disks >= 1
    error_message = "victoriametrics_data_disks must be at least 1."
  }
}

variable "victoriametrics_data_disk_gib" {
  type        = number
  default     = 12288
  description = "Size of each VictoriaMetrics data volume."

  validation {
    condition     = var.victoriametrics_data_disk_gib >= 1 && var.victoriametrics_data_disk_gib <= 16384
    error_message = "victoriametrics_data_disk_gib must be between 1 and 16384."
  }
}

variable "victoriametrics_data_disk_throughput_mbps" {
  type        = number
  default     = 500
  description = "gp3 throughput per VictoriaMetrics data volume."

  validation {
    condition     = var.victoriametrics_data_disk_throughput_mbps >= 125 && var.victoriametrics_data_disk_throughput_mbps <= 1000
    error_message = "gp3 throughput must be between 125 and 1000 MB/s."
  }
}

variable "victoriametrics_data_disk_iops" {
  type        = number
  default     = 16000
  description = "gp3 IOPS per VictoriaMetrics data volume."

  validation {
    condition     = var.victoriametrics_data_disk_iops >= 3000 && var.victoriametrics_data_disk_iops <= 16000
    error_message = "gp3 IOPS must be between 3000 and 16000."
  }
}

variable "gateway_count" {
  type        = number
  default     = 6
  description = "Gateway nodes (reference contract: 4-6)."

  validation {
    condition     = var.gateway_count >= 4 && var.gateway_count <= 6
    error_message = "gateway_count must be between 4 and 6."
  }
}

variable "gateway_instance_type" {
  type        = string
  default     = "c6i.4xlarge"
  description = "Gateway instance type (16 vCPU, 32 GiB reference)."
}

variable "gateway_data_disks" {
  type        = number
  default     = 0
  description = "Data volumes per gateway (normally zero)."

  validation {
    condition     = var.gateway_data_disks >= 0
    error_message = "gateway_data_disks must not be negative."
  }
}

variable "gateway_data_disk_gib" {
  type        = number
  default     = 0
  description = "Size of each gateway data volume."
}

variable "gateway_data_disk_throughput_mbps" {
  type        = number
  default     = 0
  description = "gp3 throughput per gateway data volume."
}

variable "gateway_data_disk_iops" {
  type        = number
  default     = 0
  description = "gp3 IOPS per gateway data volume."
}

variable "nats_count" {
  type        = number
  default     = 4
  description = "NATS JetStream nodes."

  validation {
    condition     = var.nats_count >= 3 && var.nats_count <= 5
    error_message = "nats_count must be between 3 and 5."
  }
}

variable "nats_instance_type" {
  type        = string
  default     = "m6i.2xlarge"
  description = "NATS instance type (8 vCPU, 32 GiB reference)."
}

variable "nats_data_disks" {
  type        = number
  default     = 1
  description = "Data volumes per NATS node."

  validation {
    condition     = var.nats_data_disks >= 1
    error_message = "nats_data_disks must be at least 1."
  }
}

variable "nats_data_disk_gib" {
  type        = number
  default     = 512
  description = "Size of each NATS data volume."

  validation {
    condition     = var.nats_data_disk_gib >= 1 && var.nats_data_disk_gib <= 16384
    error_message = "nats_data_disk_gib must be between 1 and 16384."
  }
}

variable "nats_data_disk_throughput_mbps" {
  type        = number
  default     = 250
  description = "gp3 throughput per NATS data volume."

  validation {
    condition     = var.nats_data_disk_throughput_mbps >= 125 && var.nats_data_disk_throughput_mbps <= 1000
    error_message = "gp3 throughput must be between 125 and 1000 MB/s."
  }
}

variable "nats_data_disk_iops" {
  type        = number
  default     = 6000
  description = "gp3 IOPS per NATS data volume."

  validation {
    condition     = var.nats_data_disk_iops >= 3000 && var.nats_data_disk_iops <= 16000
    error_message = "gp3 IOPS must be between 3000 and 16000."
  }
}

variable "loadgen_count" {
  type        = number
  default     = 2
  description = "Load generator hosts."

  validation {
    condition     = var.loadgen_count >= 1 && var.loadgen_count <= 4
    error_message = "loadgen_count must be between 1 and 4."
  }
}

variable "loadgen_instance_type" {
  type        = string
  default     = "c6i.4xlarge"
  description = "Load generator instance type (16 vCPU, 32 GiB reference)."
}

variable "loadgen_data_disks" {
  type        = number
  default     = 0
  description = "Data volumes per load generator (normally zero)."

  validation {
    condition     = var.loadgen_data_disks >= 0
    error_message = "loadgen_data_disks must not be negative."
  }
}

variable "loadgen_data_disk_gib" {
  type        = number
  default     = 0
  description = "Size of each load generator data volume."
}

variable "loadgen_data_disk_throughput_mbps" {
  type        = number
  default     = 0
  description = "gp3 throughput per load generator data volume."
}

variable "loadgen_data_disk_iops" {
  type        = number
  default     = 0
  description = "gp3 IOPS per load generator data volume."
}
