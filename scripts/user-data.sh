#!/bin/bash
# EC2 user-data bootstrap script — no Docker, runs binary directly via systemd.
# Runs once on first boot.
#
# Required SSM Parameters (set before launching):
#   /ingestion/KAFKA_BROKERS
#   /ingestion/KAFKA_TOPIC
#   /ingestion/KAFKA_DLQ_TOPIC
#   /ingestion/API_KEYS
#   /ingestion/DEBUG_SECRET
#
# Required IAM permissions on instance role:
#   s3:GetObject on your deploy bucket
#   ssm:GetParameter on /ingestion/*
#   ssm:DescribeInstanceInformation + ssm:UpdateInstanceInformation (for SSM Run Command)
#   logs:CreateLogGroup, logs:CreateLogStream, logs:PutLogEvents (CloudWatch)

set -euo pipefail

AWS_REGION="us-east-1"           # change to your region
DEPLOY_BUCKET="your-deploy-bucket"
BINARY_KEY="ingestion-server/latest/ingestion-server"

# ── System setup ───────────────────────────────────────────────────────────────
yum update -y
yum install -y aws-cli

# Dedicated non-root user to run the service
useradd -r -s /sbin/nologin ingestion

# ── Pull secrets from SSM Parameter Store ─────────────────────────────────────
get_param() {
  aws ssm get-parameter \
    --region "${AWS_REGION}" \
    --name "/ingestion/$1" \
    --with-decryption \
    --query "Parameter.Value" \
    --output text
}

mkdir -p /etc/ingestion
cat > /etc/ingestion/.env <<EOF
PORT=8080
LOG_LEVEL=info
KAFKA_BROKERS=$(get_param KAFKA_BROKERS)
KAFKA_TOPIC=$(get_param KAFKA_TOPIC)
KAFKA_DLQ_TOPIC=$(get_param KAFKA_DLQ_TOPIC)
MAX_IN_FLIGHT=4096
SHUTDOWN_TIMEOUT_SEC=15
RATE_LIMIT_RPS=10000
RATE_LIMIT_BURST=1000
API_KEYS=$(get_param API_KEYS)
DEBUG_SECRET=$(get_param DEBUG_SECRET)
EOF
chmod 600 /etc/ingestion/.env
chown ingestion:ingestion /etc/ingestion/.env

# ── Download binary from S3 ────────────────────────────────────────────────────
aws s3 cp "s3://${DEPLOY_BUCKET}/${BINARY_KEY}" /usr/local/bin/ingestion-server \
  --region "${AWS_REGION}"
chmod +x /usr/local/bin/ingestion-server

# ── Systemd service ────────────────────────────────────────────────────────────
cat > /etc/systemd/system/ingestion-server.service <<'EOF'
[Unit]
Description=Ingestion Server
After=network-online.target
Wants=network-online.target

[Service]
User=ingestion
EnvironmentFile=/etc/ingestion/.env
ExecStart=/usr/local/bin/ingestion-server
Restart=always
RestartSec=3

# Hard resource limits — prevents a runaway process from killing the instance
LimitNOFILE=65536
MemoryMax=1G

# Logging — goes to journald, shipped to CloudWatch by the agent below
StandardOutput=journal
StandardError=journal
SyslogIdentifier=ingestion-server

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable ingestion-server
systemctl start ingestion-server

# ── CloudWatch Logs agent ──────────────────────────────────────────────────────
# Ships journald logs to CloudWatch so you can view them without SSH-ing in.
yum install -y amazon-cloudwatch-agent

cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json <<EOF
{
  "logs": {
    "logs_collected": {
      "files": {
        "collect_list": [
          {
            "file_path": "/var/log/messages",
            "log_group_name": "/ingestion/server",
            "log_stream_name": "{instance_id}/messages"
          }
        ]
      }
    }
  }
}
EOF

# Forward journald → /var/log/messages so the agent picks it up
echo "ForwardToSyslog=yes" >> /etc/systemd/journald.conf
systemctl restart systemd-journald

/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl \
  -a fetch-config \
  -m ec2 \
  -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json \
  -s
