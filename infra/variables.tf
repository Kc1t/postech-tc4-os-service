variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "project_name" {
  type    = string
  default = "postech-tc4"
}

variable "environment" {
  type = string
}

variable "vpc_id" {
  type        = string
  default     = ""
  description = "VPC do banco; vazio usa a VPC padrão da conta (a mesma do cluster no Learner Lab)"
}

variable "cluster_name" {
  type        = string
  default     = ""
  description = "Cluster EKS cujos nós acessam o banco; vazio = <project>-<environment> (nome do cluster do postech-tc4-infra-k8s)"
}

variable "db_name" {
  type    = string
  default = "workshop"
}

variable "db_username" {
  type    = string
  default = "workshop_app"
}

variable "db_instance_class" {
  type    = string
  default = "db.t3.micro"
}

variable "db_allocated_storage" {
  type    = number
  default = 20
}

variable "multi_az" {
  type        = bool
  default     = false
  description = "Multi-AZ dobra o custo; no Learner Lab fica desligado (ADR-0015)"
}

variable "extra_allowed_cidr_blocks" {
  type        = list(string)
  default     = []
  description = "Faixas extras com acesso ao Postgres (por exemplo, um bastion). Os nós do EKS já entram pelo security group do cluster"
}
