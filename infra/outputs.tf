output "db_address" {
  value = aws_db_instance.this.address
}

output "db_port" {
  value = aws_db_instance.this.port
}

output "db_name" {
  value = aws_db_instance.this.db_name
}

output "db_credentials_secret_name" {
  value       = aws_secretsmanager_secret.db.name
  description = "O CI lê o campo dsn deste secret para criar o Secret do Kubernetes"
}

output "db_security_group_id" {
  value = aws_security_group.db.id
}
