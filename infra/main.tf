# Banco próprio do os-service: só ele tem a credencial e só os nós do EKS alcançam a porta.
# Nenhum outro serviço lê estas tabelas; o que eles precisam vem por mensagem ou pela API.
locals {
  name = "${var.project_name}-${var.environment}-os-service"

  tags = {
    Project     = var.project_name
    Environment = var.environment
    Service     = "os-service"
    ManagedBy   = "terraform"
  }

  vpc_id       = var.vpc_id != "" ? var.vpc_id : data.aws_vpc.default[0].id
  cluster_name = var.cluster_name != "" ? var.cluster_name : "${var.project_name}-${var.environment}"
}

data "aws_vpc" "default" {
  count   = var.vpc_id == "" ? 1 : 0
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [local.vpc_id]
  }
}

# Os nós do node group gerenciado usam o security group do cluster.
data "aws_eks_cluster" "this" {
  name = local.cluster_name
}

resource "random_password" "db" {
  length  = 24
  special = false

  # A senha só muda de propósito (terraform apply -replace), nunca como efeito de outra mudança.
  lifecycle {
    ignore_changes = all
  }
}

data "aws_kms_alias" "rds" {
  name = "alias/aws/rds"
}

# tfsec:ignore:aws-ssm-secret-use-customer-key O Learner Lab não permite criar CMK; a chave gerenciada da AWS é a única disponível.
resource "aws_secretsmanager_secret" "db" {
  name                    = "${local.name}-db-credentials"
  recovery_window_in_days = 0
  tags                    = local.tags
}

resource "aws_secretsmanager_secret_version" "db" {
  secret_id = aws_secretsmanager_secret.db.id

  secret_string = jsonencode({
    username = var.db_username
    password = random_password.db.result
    host     = aws_db_instance.this.address
    port     = aws_db_instance.this.port
    dbname   = var.db_name
    dsn      = "postgres://${var.db_username}:${random_password.db.result}@${aws_db_instance.this.address}:${aws_db_instance.this.port}/${var.db_name}?sslmode=require"
  })
}

resource "aws_db_subnet_group" "this" {
  name       = "${local.name}-db"
  subnet_ids = data.aws_subnets.default.ids
  tags       = local.tags
}

resource "aws_security_group" "db" {
  name        = "${local.name}-db"
  description = "PostgreSQL do os-service: so os nos do EKS"
  vpc_id      = local.vpc_id

  ingress {
    description     = "PostgreSQL a partir dos nos do EKS"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [data.aws_eks_cluster.this.vpc_config[0].cluster_security_group_id]
  }

  dynamic "ingress" {
    for_each = length(var.extra_allowed_cidr_blocks) > 0 ? [1] : []
    content {
      description = "PostgreSQL a partir de faixas autorizadas"
      from_port   = 5432
      to_port     = 5432
      protocol    = "tcp"
      cidr_blocks = var.extra_allowed_cidr_blocks
    }
  }

  tags = local.tags
}

resource "aws_db_parameter_group" "this" {
  name   = "${local.name}-pg16"
  family = "postgres16"

  # Conexão sem TLS é recusada pelo próprio banco.
  parameter {
    name  = "rds.force_ssl"
    value = "1"
  }

  # Consultas acima de 500 ms vão para o log (exportado ao CloudWatch).
  parameter {
    name  = "log_min_duration_statement"
    value = "500"
  }

  tags = local.tags
}

resource "aws_db_instance" "this" {
  identifier     = "${local.name}-postgres"
  engine         = "postgres"
  engine_version = "16"

  instance_class        = var.db_instance_class
  allocated_storage     = var.db_allocated_storage
  max_allocated_storage = var.db_allocated_storage * 2
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = var.db_name
  username = var.db_username
  password = random_password.db.result

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  parameter_group_name   = aws_db_parameter_group.this.name
  publicly_accessible    = false
  multi_az               = var.multi_az
  apply_immediately      = true

  backup_retention_period = 1
  skip_final_snapshot     = true

  # tfsec:ignore:aws-rds-enable-deletion-protection Desligada de propósito: o Learner Lab encerra a sessão e o ambiente é recriado por terraform.
  deletion_protection = false

  iam_database_authentication_enabled = true

  performance_insights_enabled          = true
  performance_insights_kms_key_id       = data.aws_kms_alias.rds.target_key_arn
  performance_insights_retention_period = 7

  enabled_cloudwatch_logs_exports = ["postgresql"]

  tags = local.tags

  # "16" fixa a versão maior; a menor evolui pelo auto minor upgrade e não deve virar diff.
  lifecycle {
    ignore_changes = [engine_version]
  }
}
